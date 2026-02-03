---
name: Acting
description: Self-provision missing dependencies, runtimes, or services so the agent can continue working autonomously.
---

# Instructions

Use this skill whenever the task depends on software the workspace does not already provide—missing packages, runtimes, language tooling, or service endpoints. It teaches the agent how to diagnose the gap, install what is needed, and verify the new environment before resuming higher-level work.

## When to use

- The logs mention "command not found", "module missing", or similar runtime failures for packages, binaries, or interpreters.
- The user asks for new infrastructure (Python virtualenv, Node toolchain, Docker image, etc.) before other coding work can proceed.
- An existing script or build step fails because it expects a dependency that is not installed.
- A service/daemon needs to be started (database, message queue, etc.).

## Workflow

1. **Diagnose clearly.** Inspect the failing command/output, check `which`, `go env`, `pip list`, or equivalent, and confirm the missing dependency/runtime. Capture the reason why the workspace cannot satisfy the task today.
2. **Prefer lean installation commands.** Choose the package manager that best matches the dependency (`apt`, `yum`, `brew`, `pip`, `pipx`, `npm`, `pnpm`, `go install`, etc.). When multiple managers might work, prefer the one already used in this repo (check `package.json`, `pyproject.toml`, Dockerfiles, etc.).
3. **Automate environment creation.** When the task needs an isolated environment (Python venv, Node nvm, Go workspace, etc.), create it programmatically (`python -m venv`, `nvm install`, `go env -w`, etc.), activate it, and record the commands in documentation so subsequent steps know where to find the tools.
4. **Install the dependency.** Run the install command and confirm success via version checks (e.g., `pip show`, `npm list <pkg>`, `python --version`). If installation fails, capture the error message and document troubleshooting steps.
5. **Verify installation.** After install, test the tool works with a simple command (e.g., `go version`, `python -c "import requests"`). Don't proceed until verification passes.
6. **Document the change.** Update README, setup docs, or comments with what was installed and why. Include exact commands for reproducibility.

## Decision rules

- Do not proceed with higher-level work until the required tooling can actually run locally (try the failing command once the install completes).
- When a dependency exists but has multiple versions, pick the version range the repo already expects (check lock files or docs) and justify any deviation in the notes.
- If a centralized dependency (e.g., system service) cannot be installed, explain why it is blocked and propose manual instructions the user can run.
- Always prefer project-local installs over global (e.g., `npm install` vs `npm install -g`, `pip install` in venv vs system-wide).

## Quality checks

- The new tooling must be callable from the agent's host commands (`shell.exec`, verification scripts).
- Retest the scenario that triggered the installation to prove the failure disappears.
- Documentation updated so future developers can reproduce the environment.
- No leftover artifacts from failed installation attempts.

## Common Package Managers

### System-level
- **macOS**: `brew install <package>`
- **Ubuntu/Debian**: `sudo apt-get install <package>`
- **RHEL/CentOS**: `sudo yum install <package>`
- **Alpine**: `apk add <package>`

### Language-specific
- **Python**: `pip install <package>` (or `poetry add`, `pipx install`)
- **Node.js**: `npm install <package>` (or `yarn add`, `pnpm add`)
- **Go**: `go install <package>@version`
- **Ruby**: `gem install <package>` (or `bundle add`)
- **Rust**: `cargo install <package>`

### Isolated environments
- **Python venv**: `python -m venv venv && source venv/bin/activate`
- **Node nvm**: `nvm install 18 && nvm use 18`
- **Docker**: `docker run -it <image> bash`
- **Conda**: `conda create -n myenv python=3.10`

## Templates

### Installation documentation
```markdown
## Development Setup

### Prerequisites
- Python 3.10+
- Node.js 18+
- PostgreSQL 14+

### Installation

1. **Clone repository**
   ```bash
   git clone <repo-url>
   cd <project>
   ```

2. **Install Python dependencies**
   ```bash
   python -m venv venv
   source venv/bin/activate  # Windows: venv\Scripts\activate
   pip install -r requirements.txt
   ```

3. **Install Node dependencies**
   ```bash
   npm install
   ```

4. **Start services**
   ```bash
   docker-compose up -d postgres redis
   ```

5. **Run migrations**
   ```bash
   python manage.py migrate
   ```

6. **Verify setup**
   ```bash
   make test
   ```

### Troubleshooting

**Issue**: `ModuleNotFoundError: No module named 'requests'`
**Solution**: Activate venv: `source venv/bin/activate`

**Issue**: `node: command not found`
**Solution**: Install Node via nvm: `nvm install 18`
```

## Examples

### Example 1: Missing Python package

**Error**: `ModuleNotFoundError: No module named 'requests'`

**Diagnosis**: 
```bash
python -c "import requests"  # fails
pip list | grep requests     # not found
```

**Action**:
```bash
pip install requests
python -c "import requests; print(requests.__version__)"  # 2.31.0
```

**Documentation**: Added `requests==2.31.0` to `requirements.txt`

### Example 2: Wrong Node version

**Error**: `Error: This project requires Node.js >=18`

**Diagnosis**:
```bash
node --version  # v16.20.0
```

**Action**:
```bash
nvm install 18
nvm use 18
node --version  # v18.17.0
npm install     # re-install with correct Node version
```

**Documentation**: Added `.nvmrc` with `18` to lock version

### Example 3: Missing system dependency

**Error**: `pg_config: command not found` (when installing psycopg2)

**Diagnosis**: PostgreSQL development headers not installed

**Action**:
```bash
# macOS
brew install postgresql

# Ubuntu
sudo apt-get install libpq-dev

# Verify
pg_config --version
pip install psycopg2-binary
```

**Documentation**: Added to README prerequisites

## Advanced techniques

### Containerized dev environments

Use Docker to ensure consistency:
```dockerfile
FROM python:3.10
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
CMD ["python", "app.py"]
```

### Lock files for reproducibility

Always commit lock files:
- `requirements.txt` / `poetry.lock` (Python)
- `package-lock.json` / `yarn.lock` (Node)
- `go.sum` (Go)
- `Gemfile.lock` (Ruby)

### Health checks

Add verification scripts:
```bash
#!/bin/bash
# scripts/health-check.sh

echo "Checking dependencies..."
python --version || exit 1
node --version || exit 1
psql --version || exit 1

echo "Checking Python packages..."
python -c "import django, requests, celery" || exit 1

echo "Checking services..."
pg_isready || exit 1
redis-cli ping || exit 1

echo "✅ All dependencies ready!"
```

### Dependency pinning strategies

- **Exact pins**: `requests==2.31.0` (most reproducible, hardest to update)
- **Compatible release**: `requests~=2.31` (allows patch updates)
- **Minimum version**: `requests>=2.31` (most flexible, least reproducible)

Choose based on stability needs vs. update frequency.

## Anti-patterns

- **Global installations**: Don't pollute system Python/Node - use virtual environments
- **Ignoring version conflicts**: Don't just install latest - check compatibility
- **No verification**: Don't assume it worked - always test the tool
- **Unclear documentation**: Don't leave future devs guessing - document everything
- **Kitchen sink installs**: Don't install "everything" - only what's needed

## When NOT to use

- Tool is already installed and working (just use it)
- Installation requires elevated privileges you don't have (document manual steps instead)
- Dependency is for development only and task is production-focused (defer to later)
- Alternative approach exists that doesn't need new dependencies (prefer simplicity)
