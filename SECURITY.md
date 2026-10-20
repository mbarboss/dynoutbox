# Security policy

## Supported versions

Nothing has been released yet. Only the `main` branch is supported, and fixes land
there.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:

1. Open the repository's **Security** tab.
2. Click **Report a vulnerability** and fill in the form.

Do not open a public issue or pull request for a vulnerability.

A useful report includes the affected file or package, the commit you tested, steps to
reproduce, and the impact you expect.

## What to expect

This is a personal project maintained in spare time, so responses are best effort. I will
reply to your report and agree on a fix and disclosure date with you before anything is
made public. Credit is given in the advisory unless you prefer
otherwise.

## Scope

In scope: the library code, the task runner in `tools/`, the CI workflows and, once they
exist, the examples and load-test tooling in this repository.

Out of scope: vulnerabilities in AWS services or in third-party dependencies (report
those upstream; I will update the dependency once a fix is available), and
misconfigured deployments of applications that use this library.
