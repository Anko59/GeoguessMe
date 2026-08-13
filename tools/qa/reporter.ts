import fs from 'node:fs/promises';
import type { FullConfig, FullResult, Reporter, Suite, TestCase, TestResult } from '@playwright/test/reporter';

interface Options {
    outputFile?: string;
}

interface RecordedTest {
    title: string;
    project: string;
    status: string;
    durationMs: number;
    errors: string[];
    evidence: string[];
}

export default class QAReporter implements Reporter {
    private readonly outputFile: string;
    private readonly tests: RecordedTest[] = [];
    private startedAt = '';
    private baseURL = '';

    public constructor(options: Options = {}) {
        this.outputFile = options.outputFile || '/tmp/qa-artifacts/qa-report.json';
    }

    public onBegin(config: FullConfig, suite: Suite): void {
        this.startedAt = new Date().toISOString();
        this.baseURL = config.projects[0]?.use.baseURL || process.env.QA_BASE_URL || '';
        void suite;
    }

    public onTestEnd(test: TestCase, result: TestResult): void {
        const evidence = result.attachments
            .filter((attachment) => attachment.path)
            .map((attachment) => attachment.path as string);
        this.tests.push({
            title: test.titlePath().slice(1).join(' › '),
            project: test.parent.project()?.name || 'unknown',
            status: result.status,
            durationMs: result.duration,
            errors: result.errors.map((error) => error.message || String(error)),
            evidence,
        });
    }

    public async onEnd(result: FullResult): Promise<void> {
        const completedAt = new Date().toISOString();
        const findings = this.tests
            .filter((test) => test.status !== 'passed' && test.status !== 'skipped')
            .map((test) => ({
                severity: /security|authorization|rate limit/i.test(test.title) ? 'high' : 'medium',
                journey: test.title,
                build: process.env.QA_BUILD_SHA || 'unknown',
                steps: ['Run the named journey in the recorded project.'],
                expected: 'The journey completes without a reproducible failure.',
                observed: test.errors.join('\n') || `journey ended with status ${test.status}`,
                evidence: test.evidence,
            }));
        const report = {
            schema: 'geoguessme.qa-report.v1',
            agent: 'source-blind-playwright-qa',
            build: process.env.QA_BUILD_SHA || 'unknown',
            baseURL: this.baseURL,
            startedAt: this.startedAt,
            completedAt,
            status: result.status,
            tests: this.tests,
            findings,
            limitations: [
                'Email delivery is probed only to the extent configured by the running application; no mailbox credentials are stored in this workflow.',
                'The agent does not inspect repository source code or use implementation-side test helpers.',
            ],
        };
        await fs.mkdir(new URL('.', `file://${this.outputFile}`).pathname, { recursive: true });
        await fs.writeFile(this.outputFile, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
        const markdownPath = this.outputFile.replace(/\.json$/, '.md');
        const lines = [
            '# GeoGuessMe black-box QA report',
            '',
            `- Build: \`${report.build}\``,
            `- Base URL: ${report.baseURL}`,
            `- Status: **${report.status}**`,
            `- Findings: **${findings.length}**`,
            '',
        ];
        for (const finding of findings) {
            lines.push(
                `## ${finding.severity.toUpperCase()}: ${finding.journey}`,
                '',
                `- Observed: ${finding.observed}`,
                '',
            );
        }
        if (findings.length === 0) lines.push('No reproducible defects were found by the configured journeys.', '');
        lines.push('## Scope limitations', '', ...report.limitations.map((item) => `- ${item}`), '');
        await fs.writeFile(markdownPath, lines.join('\n'), 'utf8');
    }
}
