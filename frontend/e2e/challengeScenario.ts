import { expect, type Browser, type BrowserContextOptions, type Page } from '@playwright/test';
import {
    installDeterministicCamera,
    installDeterministicGeolocation,
    newAuthContext,
    signupViaUI,
    uniqueGroup,
    expectConnected,
} from './helpers';

export interface Scenario {
    uploader: Page;
    guesser: Page;
    uploaderContext: Awaited<ReturnType<typeof newAuthContext>>;
    guesserContext: Awaited<ReturnType<typeof newAuthContext>>;
}

export function cameraOptions(contextOptions: BrowserContextOptions): BrowserContextOptions {
    return {
        ...contextOptions,
        permissions: ['camera', 'geolocation'],
        geolocation: { latitude: 48.8566, longitude: 2.3522 },
        baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080',
    };
}

export async function createScenario(browser: Browser, contextOptions: BrowserContextOptions): Promise<Scenario> {
    const options = cameraOptions(contextOptions);
    const uploaderContext = await newAuthContext(browser, options);
    const guesserContext = await newAuthContext(browser, options);
    await installDeterministicCamera(uploaderContext);
    await installDeterministicGeolocation(uploaderContext);
    const uploader = await uploaderContext.newPage();
    await signupViaUI(uploader);
    await uploader.goto('/group/create');
    await uploader.getByPlaceholder('Group Name').fill(uniqueGroup());
    await uploader.locator('form.join-form').getByRole('button', { name: 'Create Group' }).click();
    await uploader.waitForURL(/\/group\/[0-9a-f-]{36}$/);
    const groupId = uploader.url().split('/group/')[1];
    await uploader.getByRole('button', { name: 'Open group settings' }).click();
    const settings = uploader.getByRole('dialog');
    const groupCode = (await settings.locator('.group-code').textContent())?.trim() ?? '';
    await settings.getByRole('button', { name: 'Close settings' }).click();

    const guesser = await guesserContext.newPage();
    await signupViaUI(guesser);
    await guesser.goto('/group/join');
    await guesser.getByPlaceholder('6-character code').fill(groupCode);
    await guesser.locator('form.join-form').getByRole('button', { name: 'Join Group' }).click();
    await guesser.waitForURL(/\/group\//);

    await uploader.goto('/group/' + groupId);
    await guesser.goto('/group/' + groupId);
    await expectConnected(uploader);
    await expectConnected(guesser);
    return { uploader, guesser, uploaderContext, guesserContext };
}

export async function closeScenario(scenario: Scenario): Promise<void> {
    await scenario.uploaderContext.close();
    await scenario.guesserContext.close();
}
