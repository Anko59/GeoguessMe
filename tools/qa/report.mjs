import { mkdir, writeFile } from "node:fs/promises";

export async function writeQaReport({ artifactDir, hostArtifactDir, startedAt, baseUrl, runtime, budget, buildSha, mailboxProvider, geolocation, mailboxCount, summary, findings, artifacts }) {
  await mkdir(artifactDir, { recursive: true });
  const report = {
    schema_version: 1,
    started_at: startedAt,
    finished_at: new Date().toISOString(),
    target: { base_url: `${baseUrl.origin}${baseUrl.pathname}`, build_sha: buildSha },
    runtime,
    budget,
    status: summary.status || (findings.length ? "FINDINGS" : "BLOCKED"),
    summary: summary.summary || "The runtime ended before qa_finish was called.",
    journeys_exercised: summary.journeys_exercised || [],
    journeys_not_exercised: summary.journeys_not_exercised || [],
    limitations: summary.limitations || [],
    capabilities: { camera_mock: true, geolocation_mock: geolocation, mailbox_provider: mailboxProvider, mailboxes_created: mailboxCount },
    findings,
    artifacts,
  };
  await writeFile(`${artifactDir}/qa-report.json`, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
  return { report_path: `${hostArtifactDir}/qa-report.json`, finding_count: findings.length, blocking_finding_count: findings.filter((finding) => finding.blocking).length };
}
