# GitLab CI/CD Pipeline Documentation

This document explains the GitLab CI/CD pipeline for the Firecracker Agent project.

## Overview

The pipeline automates:
- **Code Quality**: Linting and static analysis
- **Testing**: Unit and integration tests with coverage
- **Building**: Binary compilation and protobuf generation
- **Packaging**: Debian package creation for Debian Trixie

## Pipeline Stages

### 1. Lint Stage

Ensures code quality and consistency.

#### Jobs:

**lint:format**
- Checks Go code formatting with `gofmt`
- Fails if any files need formatting
- Runs on: MRs, main, develop

**lint:vet**
- Runs `go vet` to detect suspicious constructs
- Fails on any issues found
- Runs on: MRs, main, develop

**lint:staticcheck**
- Advanced static analysis with staticcheck
- Allowed to fail (warnings only)
- Runs on: MRs, main, develop

### 2. Test Stage

Runs comprehensive test suite.

#### Jobs:

**test:unit**
- Runs all unit tests with race detection
- Generates coverage report (HTML + console)
- Installs qemu-utils for storage tests
- Coverage threshold: tracked in GitLab
- Artifacts: coverage.out, coverage.html (30 days)
- Runs on: MRs, main, develop

**test:integration**
- Runs integration tests (tagged with `integration`)
- Requires Docker-in-Docker
- Installs network tools (iproute2, iptables, bridge-utils)
- Allowed to fail (requires special setup)
- Runs on: MRs, main, develop

### 3. Build Stage

Compiles the application.

#### Jobs:

**build:protobuf**
- Generates Go code from protobuf definitions
- Installs protoc compiler and Go plugins
- Artifacts: *.pb.go files (1 hour)
- Runs on: MRs, main, develop, tags

**build:binary**
- Compiles the fc-agent binary
- Embeds version and build time
- Static linking (CGO_ENABLED=0)
- Artifacts: bin/fc-agent (7 days)
- Runs on: MRs, main, develop, tags

Build flags:
```bash
-ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"
```

### 4. Package Stage

Creates distribution packages.

#### Jobs:

**package:debian**
- Builds Debian package (.deb) for Debian Trixie
- Uses Debian Trixie image
- Runs dpkg-buildpackage
- Runs lintian checks (warnings allowed)
- Generates: .deb, .buildinfo, .changes files
- Artifacts: packages/* (30 days)
- Runs on: main, develop, tags only

Package output:
```
firecracker-agent_0.1.0-1_amd64.deb
firecracker-agent_0.1.0-1_amd64.buildinfo
firecracker-agent_0.1.0-1_amd64.changes
```

## Required GitLab CI/CD Variables

Configure these in Settings → CI/CD → Variables (optional):

| Variable | Default | Description |
|----------|---------|-------------|
| `PACKAGE_VERSION` | 0.1.0 | Package version number |
| `PACKAGE_REVISION` | 1 | Debian package revision |

## Caching Strategy

The pipeline caches:
- `.cache/go-build` - Go build cache
- `go/pkg/mod` - Go module cache

This speeds up subsequent pipeline runs significantly.

## Artifacts

### Unit Tests
- **Duration**: 30 days
- **Files**: coverage.out, coverage.html
- **Size**: ~1-5 MB

### Binary Build
- **Duration**: 7 days
- **Files**: bin/fc-agent
- **Size**: ~15-20 MB

### Debian Package
- **Duration**: 30 days
- **Files**: *.deb, *.buildinfo, *.changes
- **Size**: ~10-15 MB

## Branch Strategy

### Main Branch
- Full pipeline runs
- Builds Debian package
- Can deploy to production (manual)
- Creates releases on tags

### Develop Branch
- Full pipeline runs
- Builds Debian package
- Can deploy to staging (manual)

### Merge Requests
- Lint, test, and build only
- No packaging or deployment
- Requires passing tests to merge

### Tags
- Full pipeline runs
- Builds Debian package
- Creates GitHub release
- Production deployment available

## Usage Examples

### Running the Pipeline

**On Feature Branch:**
```bash
git checkout -b feature/new-feature
# ... make changes ...
git commit -m "feat: add new feature"
git push origin feature/new-feature
# Create MR - pipeline runs automatically
```

**Creating a Release:**
```bash
git checkout main
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
# Pipeline runs and builds package
```

### Downloading Artifacts

**From Pipeline:**
1. Go to CI/CD → Pipelines
2. Click on pipeline
3. Click "Browse" next to the job
4. Download files

**Direct Link:**
```
https://gitlab.com/your-org/firecracker-agent/-/jobs/[JOB_ID]/artifacts/download
```

## Troubleshooting

### Pipeline Fails on Lint

**Problem**: Code is not formatted
```bash
# Fix locally
gofmt -w .
git commit -am "fix: format code"
git push
```

### Tests Fail

**Problem**: Tests don't pass
```bash
# Run tests locally
go test -v -race ./...

# Check specific test
go test -v -run TestName ./path/to/package

# Fix issues and commit
```

### Debian Package Build Fails

**Problem**: Build dependencies missing
- Check debian/control for all dependencies
- Verify debian/rules build process
- Test locally: `./build-deb.sh`

### Deployment Fails

**Problem**: SSH authentication
- Verify SSH_PRIVATE_KEY is correct
- Check SSH_KNOWN_HOSTS contains server
- Verify DEPLOY_USER has permissions
- Test SSH manually: `ssh DEPLOY_USER@SERVER`

**Problem**: rsync fails
```bash
# Check if rsync is installed on server
ssh DEPLOY_USER@SERVER "which rsync"

# Check deployment path exists
ssh DEPLOY_USER@SERVER "ls -la /opt/packages"
```

### Coverage Not Reported

**Problem**: Coverage report not showing in GitLab
- Check coverage regex in .gitlab-ci.yml
- Verify coverage.out is generated
- Check GitLab Settings → CI/CD → General pipelines → Test coverage parsing

## Local Testing

Before pushing, test locally:

```bash
# Format code
gofmt -w .

# Run linters
go vet ./...
staticcheck ./... 2>/dev/null || true

# Run tests
go test -v -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1

# Build binary
go build -o bin/fc-agent ./cmd/fc-agent

# Build Debian package
./build-deb.sh
```

## Pipeline Optimization

### Speed Improvements

1. **Use cache effectively**
   - Go module cache reduces download time
   - Build cache speeds up compilation

2. **Parallel jobs**
   - Lint jobs run in parallel
   - Test jobs run in parallel
   - Build jobs run in parallel

3. **Conditional execution**
   - Package only on main/develop/tags
   - Deploy only when needed

### Cost Optimization

1. **Artifacts retention**
   - Short retention for MR artifacts (7 days)
   - Longer retention for releases (30 days)

2. **Job conditions**
   - Skip unnecessary jobs on MRs
   - Only run full suite on important branches

## Advanced Configuration

### Running Only Specific Jobs

Use GitLab's `only` and `except` keywords:

```yaml
job:
  only:
    - main
    - /^release-.*$/  # Regex for release branches
  except:
    - schedules
```

### Manual Jobs

Some jobs require manual trigger:
```yaml
deploy:production:
  when: manual
```

### Retry Failed Jobs

Configure automatic retry:
```yaml
test:unit:
  retry:
    max: 2
    when: runner_system_failure
```

## Monitoring

### Pipeline Success Rate

Track in: CI/CD → Pipelines

Key metrics:
- Success rate per branch
- Average duration
- Failed jobs

### Coverage Trends

Track in: Analytics → Repository Analytics → Test Coverage

- Coverage percentage over time
- Coverage by package
- Untested code

## Best Practices

1. **Always run tests locally** before pushing
2. **Keep pipelines fast** - optimize slow jobs
3. **Use meaningful commit messages** - triggers proper pipeline stages
4. **Tag releases properly** - semantic versioning
5. **Review artifacts** before deploying
6. **Monitor pipeline failures** and fix promptly
7. **Keep dependencies updated** in debian/control
8. **Document pipeline changes** in commit messages

## Security Considerations

1. **SSH Keys**: Use dedicated deployment keys with minimal permissions
2. **Secrets**: Store in GitLab CI/CD variables (masked)
3. **Artifacts**: Set appropriate expiration times
4. **Deployment**: Always require manual approval for production
5. **Package Signing**: Consider GPG signing for packages (future enhancement)

## Future Enhancements

- [ ] Add security scanning (gosec, trivy)
- [ ] Add dependency vulnerability scanning
- [ ] Add GPG package signing
- [ ] Add automatic version bumping
- [ ] Add changelog generation
- [ ] Add Docker image building
- [ ] Add performance benchmarks
- [ ] Add notification on deployment (Slack/Email)
