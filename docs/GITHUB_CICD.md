# GitHub CI/CD and Update Flow

This repository uses GitHub Actions for continuous integration, continuous delivery, and safe automated dependency updates.

## CI

The `CI` workflow runs on every push and pull request and performs:

- `go mod download`
- `make ci-fmt-check`
- `go vet ./...`
- `make ci-test`
- `make ci-build`
- `docker build -f docker/Dockerfile.production ...`

The test suite intentionally covers a maintained subset of features instead of every project path. Today that subset is:

- `./app/services`
- `./app/scheduler`

This gives us stable automated coverage for token handling and scheduler/provider logic without introducing database-heavy or environment-dependent checks into every commit.

## CD

The `CD` workflow does not deploy to your server automatically.

Instead, after a successful `CI` run on `main` or `master`, it:

- builds the production image
- pushes the image to `ghcr.io/<owner>/yamata-no-orochi`
- uploads a `deployment-manifest` artifact containing the commit SHA, published tags, and image digest

This matches the current operational model where deployment is still performed manually on the server by pulling the repository, building or selecting the image, and running the scripts in `scripts/`.

## Update

Automated updates are handled with Dependabot:

- Go modules: weekly
- Docker dependencies: weekly
- GitHub Actions: monthly

Patch and minor Dependabot pull requests are configured for auto-merge after required checks pass. Major updates still stay manual for safety.

## Recommended GitHub Settings

In the GitHub repository settings, enable the following:

1. Packages permission for GitHub Actions so the `CD` workflow can push to `GHCR`.
2. Branch protection on `main` or `master`.
3. Required status checks:
   - `CI / verify`
4. Require pull request reviews for non-Dependabot changes if you want stricter release control.

## Manual Deployment After CD

Your deployment flow remains:

1. Pull the latest code on the server.
2. Optionally pull the published GHCR image, or build locally with:
   `docker build -f docker/Dockerfile.production -t yamata-no-orochi:local .`
3. Run the appropriate script from `scripts/`, such as `scripts/deploy-beta.sh`.

That gives you automated verification and packaging in GitHub, while keeping the final production rollout in your control.
