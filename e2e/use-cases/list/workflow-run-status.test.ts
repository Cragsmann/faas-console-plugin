import { test, expect } from '../../fixtures/authenticated-page';
import { navigateToFunctionsList } from '../../helpers/navigation';
import {
  deleteRepoOnFakeGithub,
  resetFakeGithub,
  seedRepo,
  setWorkflowRun,
} from '../../helpers/fakegithub';
import {
  simulateGitHubActionsDeploy,
  deleteFunction,
  deleteNamespace,
  ensureNamespace,
} from '../../helpers/cluster';
import { E2E_USER, PRESEEDED_FUNC_NAMESPACE } from '../../helpers/constants';

const FUNC_NAME = 'workflow-status-func';
const BRANCH = 'main';
// Deliberately never created on the cluster.
const MISSING_NAMESPACE = 'workflow-status-missing-ns';

test.describe('Workflow run status', () => {
  test.beforeEach(async ({ page }) => {
    // The namespace must exist or the backend reports a namespace error, which
    // takes over the status cell this suite asserts on.
    await ensureNamespace(page, PRESEEDED_FUNC_NAMESPACE);
    await seedRepo(
      E2E_USER,
      FUNC_NAME,
      BRANCH,
      ['serverless-function'],
      [
        {
          path: 'func.yaml',
          mode: '100644',
          content: `name: ${FUNC_NAME}\nruntime: node\nnamespace: ${PRESEEDED_FUNC_NAMESPACE}\n`,
        },
      ],
    );
  });

  test.afterEach(async () => {
    await deleteRepoOnFakeGithub(E2E_USER, FUNC_NAME);
  });

  test.afterAll(async () => {
    await resetFakeGithub();
  });

  test('shows a workflow failure indicator when the deploy workflow fails', async ({ page }) => {
    await test.step('trigger a workflow run', async () => {
      await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
        headSha: 'sha-in-progress',
        status: 'in_progress',
      });
    });

    await test.step('verify the function shows a progress indicator', async () => {
      await navigateToFunctionsList(page);
      const grid = page.getByRole('grid', { name: 'Functions' });
      await expect(grid).toBeVisible({ timeout: 30_000 });

      const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
      await expect(row).toBeVisible();
      // NotDeployed is always primary; workflow activity is a secondary indicator
      await expect(row.getByText('Not deployed')).toBeVisible({ timeout: 20_000 });
      await expect(row.getByRole('img', { name: /workflow run in progress/i })).toBeVisible({
        timeout: 20_000,
      });
    });

    await test.step('verify the failure indicator appears after the workflow fails', async () => {
      // The list streams over SSE (~3s poll cadence), so the status should update
      // without a manual refresh.
      await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
        headSha: 'sha-failed',
        status: 'completed',
        conclusion: 'failure',
      });

      const grid = page.getByRole('grid', { name: 'Functions' });
      const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
      // NotDeployed stays primary; failure shown as secondary indicator with link
      await expect(row.getByText('Not deployed')).toBeVisible({ timeout: 20_000 });
      const runLink = row.locator('a[href*="/actions/runs/"]');
      await expect(runLink).toBeVisible();
    });
  });

  test('clears the progress indicator after the workflow succeeds', async ({ page }) => {
    await test.step('trigger a workflow run', async () => {
      await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
        headSha: 'sha-in-progress',
        status: 'in_progress',
      });
    });

    await test.step('verify the progress indicator is visible', async () => {
      await navigateToFunctionsList(page);
      const grid = page.getByRole('grid', { name: 'Functions' });
      await expect(grid).toBeVisible({ timeout: 30_000 });

      const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
      await expect(row.getByRole('img', { name: /workflow run in progress/i })).toBeVisible({
        timeout: 20_000,
      });
    });

    await test.step('verify the indicator disappears after the workflow succeeds', async () => {
      await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
        headSha: 'sha-success',
        status: 'completed',
        conclusion: 'success',
      });

      const grid = page.getByRole('grid', { name: 'Functions' });
      const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
      // Spinner disappears, NotDeployed remains (no stale Deploying)
      await expect(row.getByRole('img', { name: /workflow run in progress/i })).not.toBeVisible({
        timeout: 20_000,
      });
      await expect(row.getByText('Not deployed')).toBeVisible();
    });
  });

  test('preserves running status when a redeployment workflow fails', async ({ page }) => {
    await test.step('deploy the function to the cluster', async () => {
      await simulateGitHubActionsDeploy(page, FUNC_NAME, PRESEEDED_FUNC_NAMESPACE, 'node');
    });

    await test.step('fail a redeployment workflow', async () => {
      await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
        headSha: 'sha-redeploy-failed',
        status: 'completed',
        conclusion: 'failure',
      });
    });

    await test.step('verify the function is still running with a failure indicator', async () => {
      await navigateToFunctionsList(page);
      const grid = page.getByRole('grid', { name: 'Functions' });
      await expect(grid).toBeVisible({ timeout: 30_000 });

      const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
      // Running is preserved (non-destructive); failure is a secondary indicator
      await expect(row.getByText('Running')).toBeVisible({ timeout: 20_000 });
      const runLink = row.locator('a[href*="/actions/runs/"]');
      await expect(runLink).toBeVisible({ timeout: 20_000 });
    });

    await test.step('cleanup', async () => {
      await deleteFunction(page, FUNC_NAME, PRESEEDED_FUNC_NAMESPACE);
    });
  });

  // A repo-level error and a workflow indicator were introduced by separate
  // features and had only ever been exercised apart. A missing namespace is
  // the case where they meet on a function whose func.yaml parsed cleanly, so
  // the entry keeps its name and still has to pick up its workflow run.
  test.describe('with the namespace missing from the cluster', () => {
    test('shows the namespace error as primary with the build indicator beside it', async ({
      page,
    }) => {
      // Point func.yaml at a namespace that is never created instead of deleting
      // the shared one. Deleting it would have to wait for the finalizers of
      // everything earlier specs deployed there, which blows past the helper's
      // own 120s budget in a full run, and it would leave the rest of the suite
      // to rebuild the baseline.
      await seedRepo(
        E2E_USER,
        FUNC_NAME,
        BRANCH,
        ['serverless-function'],
        [
          {
            path: 'func.yaml',
            mode: '100644',
            content: `name: ${FUNC_NAME}\nruntime: node\nnamespace: ${MISSING_NAMESPACE}\n`,
          },
        ],
      );
      // Cheap guard: a no-op unless a previous run somehow left it behind.
      await deleteNamespace(page, MISSING_NAMESPACE);

      await test.step('trigger a workflow run', async () => {
        await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
          headSha: 'sha-in-progress',
          status: 'in_progress',
        });
      });

      await test.step('verify the function shows an error with a progress indicator', async () => {
        await navigateToFunctionsList(page);
        const grid = page.getByRole('grid', { name: 'Functions' });
        await expect(grid).toBeVisible({ timeout: 30_000 });

        const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
        // The entry keeps its name, so it stays a single row keyed on it
        await expect(row).toHaveCount(1);
        await expect(row.getByText('Error')).toBeVisible({ timeout: 20_000 });
        await expect(row.getByRole('img', { name: /workflow run in progress/i })).toBeVisible({
          timeout: 20_000,
        });
      });

      await test.step('hover Error chip shows the namespace error tooltip', async () => {
        const grid = page.getByRole('grid', { name: 'Functions' });
        const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
        await row.getByText('Error').hover();
        await expect(page.getByRole('tooltip')).toContainText(
          'func.yaml: namespace does not exist',
        );
      });

      await test.step('verify the error persists after the workflow fails', async () => {
        await setWorkflowRun(E2E_USER, FUNC_NAME, BRANCH, {
          headSha: 'sha-failed',
          status: 'completed',
          conclusion: 'failure',
        });

        const grid = page.getByRole('grid', { name: 'Functions' });
        const row = grid.locator(`tbody tr:has(td:text-is("${FUNC_NAME}"))`);
        // Spinner gives way to the failure link, Error stays primary throughout
        const runLink = row.locator('a[href*="/actions/runs/"]');
        await expect(runLink).toBeVisible({ timeout: 20_000 });
        await expect(row.getByText('Error')).toBeVisible();
        await expect(row).toHaveCount(1);
      });
    });
  });
});
