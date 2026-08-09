## Summary

Describe the behavior changed and why.

## Design fit

Name the owning module and explain why the change belongs there. Call out any
new dependency, abstraction, configuration, or compatibility path; write `None`
when there is none.

## Adjacent maintenance

Report the result of the bounded maintenance scan. List small cleanups included
in this PR and deferred findings with an issue or durable ledger reference.
Write `No findings` when the scan found nothing.

## Test impact

Paste the relevant `make impact BASE=<base-ref>` summary, then list the focused
and aggregate gates required by that impact. Explain any additional gate run.

## Validation

List the Dockerized Make targets run and their results.

## Contribution checklist

- [ ] This PR targets `dev`, or it is a `release/*` PR to `main` whose tree
      exactly matches the successfully deployed `dev` tree.
- [ ] Behavior changes include deterministic regression coverage.
- [ ] The owning module and design fit are documented above.
- [ ] The bounded maintenance scan was completed; deferred debt has a durable
      reference instead of an unowned TODO.
- [ ] Documentation and operational assumptions are updated where relevant.
- [ ] Commits intended for protected branches will be GitHub-verified through
      the required squash merge.
- [ ] No secrets, generated reports, caches, test data, or plaintext environment
      files are included.
