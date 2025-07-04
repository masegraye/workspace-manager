# WSM V2 Commands QA Testing Checklist

## Overview

This document provides a comprehensive quality assurance checklist for testing all 15 migrated WSM v2 commands in real-world scenarios. The migration transformed WSM from a 1,862-line monolithic architecture to a clean service-based architecture with dependency injection.

**Migration Status**: ✅ COMPLETE (15/15 commands migrated)

---

## Pre-Testing Setup

### 1. Environment Preparation

```bash
# Build the latest version
cd workspace-manager
go build ./cmd/wsm

# Verify all commands are available
./wsm --help | grep -E "v2$"

# Expected: Should see all 15 v2 commands listed
```

### 2. Test Repository Setup

Create test repositories for comprehensive testing:

```bash
# Create test directories
mkdir -p ~/qa-test-repos
cd ~/qa-test-repos

# Create 3 test git repositories
for repo in test-repo-a test-repo-b test-repo-c; do
    mkdir $repo
    cd $repo
    git init
    echo "# $repo" > README.md
    git add README.md
    git commit -m "Initial commit"
    git branch -M main
    cd ..
done

# Create a Go module repo for go.work testing
mkdir test-go-repo
cd test-go-repo
git init
go mod init github.com/test/test-go-repo
echo 'package main\n\nfunc main() {\n\tprintln("Hello World")\n}' > main.go
git add .
git commit -m "Initial Go project"
git branch -M main
cd ..
```

### 3. Backup Existing Configuration

```bash
# Backup existing WSM configuration
cp -r ~/.config/workspace-manager ~/.config/workspace-manager.backup
```

---

## Core Command Testing

### ✅ 1. Discovery Commands

#### 1.1 discover-v2

**Test Repository Discovery:**
```bash
# Test basic discovery
./wsm discover-v2 ~/qa-test-repos --recursive

# Expected: Should find 4 repositories
# Verify: Check registry file created at ~/.config/wsm/registry.json
```

**Test Discovery with Filtering:**
```bash
# Test with specific paths
./wsm discover-v2 ~/qa-test-repos/test-repo-a ~/qa-test-repos/test-repo-b

# Test dry-run
./wsm discover-v2 ~/qa-test-repos --dry-run

# Expected: Should show what would be discovered without saving
```

**❌ Error Cases to Test:**
```bash
# Test non-existent directory
./wsm discover-v2 /nonexistent/path

# Expected: Should show clear error message
```

#### 1.2 list-v2

**Test Repository Listing:**
```bash
# List all discovered repositories
./wsm list-v2 repos

# Test JSON output
./wsm list-v2 repos --format json

# Test tag filtering (after adding tags manually to registry)
./wsm list-v2 repos --tags golang,test
```

**Test Workspace Listing:**
```bash
# List workspaces (should be empty initially)
./wsm list-v2 workspaces

# Test JSON format
./wsm list-v2 workspaces --format json
```

### ✅ 2. Workspace Creation Commands

#### 2.1 create-v2

**Test Basic Workspace Creation:**
```bash
# Create workspace with discovered repositories
./wsm create-v2 qa-test-workspace --repos test-repo-a,test-repo-b

# Expected: 
# - Workspace directory created
# - Git worktrees set up
# - go.work file created if Go repos detected
# - AGENT.md file created
```

**Test Advanced Creation Options:**
```bash
# Create with custom branch
./wsm create-v2 qa-feature-branch --repos test-repo-a,test-go-repo --branch feature/qa-test

# Create with branch prefix
./wsm create-v2 qa-prefix-test --repos test-repo-a --branch-prefix bug

# Test dry-run
./wsm create-v2 qa-dry-run --repos test-repo-a --dry-run

# Expected: Should show plan without creating workspace
```

**Test Interactive Mode:**
```bash
# Test interactive repository selection
./wsm create-v2 qa-interactive --interactive

# Expected: Should prompt for repository selection
```

**❌ Error Cases to Test:**
```bash
# Test with non-existent repository
./wsm create-v2 qa-error --repos nonexistent-repo

# Test duplicate workspace name
./wsm create-v2 qa-test-workspace --repos test-repo-a

# Expected: Should show appropriate error messages
```

#### 2.2 fork-v2

**Test Workspace Forking:**
```bash
# Create base workspace first
./wsm create-v2 qa-base-workspace --repos test-repo-a,test-repo-b

# Fork the workspace
cd ~/workspaces/*/qa-base-workspace  # Navigate to workspace
./wsm fork-v2 qa-forked-workspace

# Test with custom branch
./wsm fork-v2 qa-forked-custom --branch feature/forked-custom

# Test dry-run
./wsm fork-v2 qa-fork-dry-run --dry-run
```

**❌ Error Cases to Test:**
```bash
# Test fork from outside workspace
cd ~
./wsm fork-v2 qa-fork-error

# Expected: Should detect no workspace in current directory
```

### ✅ 3. Information Commands

#### 3.1 info-v2

**Test Workspace Information:**
```bash
# Get info for specific workspace
./wsm info-v2 qa-test-workspace

# Get info from within workspace directory
cd ~/workspaces/*/qa-test-workspace
./wsm info-v2

# Test JSON output
./wsm info-v2 --format json

# Test specific field output
./wsm info-v2 --field path
./wsm info-v2 --field repositories
./wsm info-v2 --field branch
```

**❌ Error Cases to Test:**
```bash
# Test non-existent workspace
./wsm info-v2 nonexistent-workspace

# Test from non-workspace directory
cd ~
./wsm info-v2
```

#### 3.2 status-v2

**Test Workspace Status:**
```bash
# Get status for workspace
cd ~/workspaces/*/qa-test-workspace
./wsm status-v2

# Test specific workspace
./wsm status-v2 qa-test-workspace

# Test JSON output
./wsm status-v2 --format json

# Make some changes and test status
echo "Modified content" >> test-repo-a/README.md
./wsm status-v2

# Expected: Should show modified files
```

### ✅ 4. Repository Management Commands

#### 4.1 add-v2

**Test Adding Repositories:**
```bash
# Add single repository to workspace
cd ~/workspaces/*/qa-test-workspace
./wsm add-v2 test-repo-c

# Add multiple repositories
./wsm add-v2 test-go-repo --repos additional-repo

# Test with custom branch
./wsm add-v2 test-repo-extra --branch feature/add-test

# Test dry-run
./wsm add-v2 test-repo-dry --dry-run

# Verify go.work updated for Go repositories
cat go.work
```

**❌ Error Cases to Test:**
```bash
# Test adding non-existent repository
./wsm add-v2 nonexistent-repo

# Test adding already existing repository
./wsm add-v2 test-repo-a
```

#### 4.2 remove-v2

**Test Removing Repositories:**
```bash
# Remove single repository
cd ~/workspaces/*/qa-test-workspace
./wsm remove-v2 test-repo-c

# Remove multiple repositories
./wsm remove-v2 test-repo-a test-repo-b

# Test with force (if uncommitted changes)
echo "Uncommitted change" >> test-go-repo/main.go
./wsm remove-v2 test-go-repo --force

# Test dry-run
./wsm remove-v2 remaining-repo --dry-run

# Test with file removal
./wsm remove-v2 some-repo --remove-files
```

### ✅ 5. Git Operations Commands

#### 5.1 branch-v2

**Test Branch Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# List current branches
./wsm branch-v2 list

# Create new branch across repositories
./wsm branch-v2 create feature/qa-branch-test

# Switch to existing branch
./wsm branch-v2 switch main
./wsm branch-v2 switch feature/qa-branch-test

# Test with tracking
./wsm branch-v2 create feature/tracked-branch --track
```

**❌ Error Cases to Test:**
```bash
# Test switching to non-existent branch
./wsm branch-v2 switch nonexistent-branch

# Test creating branch that already exists
./wsm branch-v2 create main
```

#### 5.2 commit-v2

**Test Commit Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Make changes in multiple repositories
echo "QA test change" >> test-repo-a/README.md
echo "Another QA change" >> test-repo-b/README.md

# Add all changes and commit
./wsm commit-v2 --add-all -m "QA: Test commit across repositories"

# Test commit with template
./wsm commit-v2 --add-all --template feature -m "Add new QA feature"

# Test selective repository commits
echo "Selective change" >> test-repo-a/README.md
./wsm commit-v2 --repositories test-repo-a -m "Selective commit test"

# Test dry-run
echo "Dry run change" >> test-repo-b/README.md
./wsm commit-v2 --add-all --dry-run -m "Dry run test"

# Test JSON output
./wsm commit-v2 --add-all --format json -m "JSON output test"
```

#### 5.3 push-v2

**Test Push Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Basic push (requires remote setup)
# Note: This will fail unless remotes are configured
./wsm push-v2

# Test specific remote
./wsm push-v2 --remote origin

# Test force push
./wsm push-v2 --force

# Test dry-run
./wsm push-v2 --dry-run

# Test JSON output
./wsm push-v2 --format json
```

#### 5.4 rebase-v2

**Test Rebase Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Test rebase all repositories
./wsm rebase-v2 --target main

# Test rebase specific repository
./wsm rebase-v2 test-repo-a --target main

# Test interactive rebase
./wsm rebase-v2 test-repo-a --interactive

# Test dry-run
./wsm rebase-v2 --dry-run --target main
```

#### 5.5 sync-v2

**Test Sync Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Basic sync
./wsm sync-v2

# Sync specific workspace
./wsm sync-v2 qa-test-workspace

# Test with rebase
./wsm sync-v2 --rebase

# Test fetch only
./wsm sync-v2 --fetch-only

# Test JSON output
./wsm sync-v2 --format json
```

#### 5.6 merge-v2

**Test Merge Operations:**
```bash
# Create workspace to merge
./wsm create-v2 qa-merge-source --repos test-repo-a --branch feature/merge-test

# Create target workspace
./wsm create-v2 qa-merge-target --repos test-repo-a --branch main

# Test merge
cd ~/workspaces/*/qa-merge-source
./wsm merge-v2 qa-merge-target

# Test dry-run
./wsm merge-v2 qa-merge-target --dry-run

# Test force merge
./wsm merge-v2 qa-merge-target --force

# Test keep workspace
./wsm merge-v2 qa-merge-target --keep-workspace
```

### ✅ 6. Workspace Management Commands

#### 6.1 delete-v2

**Test Workspace Deletion:**
```bash
# Delete configuration only
./wsm delete-v2 qa-forked-workspace

# Delete with all files
./wsm delete-v2 qa-merge-source --remove-files

# Force delete without confirmation
./wsm delete-v2 qa-merge-target --force --remove-files

# Test with uncommitted changes
cd ~/workspaces/*/qa-test-workspace
echo "Uncommitted" >> test-repo-a/README.md
cd ~
./wsm delete-v2 qa-test-workspace --force-worktrees

# Test JSON output
./wsm delete-v2 remaining-workspace --output json
```

### ✅ 7. Integration Commands

#### 7.1 tmux-v2

**Test Tmux Integration:**
```bash
# Create workspace with tmux config
cd ~/workspaces/*/qa-test-workspace

# Create basic tmux config
mkdir -p .wsm
cat > .wsm/tmux.conf << 'EOF'
# Basic tmux configuration
new-session -d -s workspace
split-window -h
send-keys -t 0 'echo "Left pane"' Enter
send-keys -t 1 'echo "Right pane"' Enter
EOF

# Test tmux session creation
./wsm tmux-v2

# Test with profile
mkdir -p .wsm/profiles/dev
cat > .wsm/profiles/dev/tmux.conf << 'EOF'
new-session -d -s dev-session
send-keys 'echo "Development session"' Enter
EOF

./wsm tmux-v2 --profile dev
```

#### 7.2 starship-v2

**Test Starship Integration:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Generate starship config
./wsm starship-v2

# Test with custom options
./wsm starship-v2 --symbol "🚀" --style "bold blue"

# Test with date
./wsm starship-v2 --show-date

# Test force overwrite
./wsm starship-v2 --force

# Verify config was added to ~/.config/starship.toml
tail ~/.config/starship.toml
```

#### 7.3 diff-v2

**Test Diff Operations:**
```bash
cd ~/workspaces/*/qa-test-workspace

# Make changes for testing
echo "Change for diff" >> test-repo-a/README.md
echo "Another change" >> test-repo-b/README.md

# Test workspace diff
./wsm diff-v2

# Test staged diff
git add test-repo-a/README.md
./wsm diff-v2 --staged

# Test specific repository
./wsm diff-v2 --repo test-repo-a

# Test from workspace path
./wsm diff-v2 --workspace ~/workspaces/*/qa-test-workspace
```

---

## Integration Testing Scenarios

### Scenario 1: Complete Workflow

Test a complete development workflow using only v2 commands:

```bash
# 1. Discover repositories
./wsm discover-v2 ~/qa-test-repos --recursive

# 2. Create workspace
./wsm create-v2 qa-workflow --repos test-repo-a,test-repo-b,test-go-repo

# 3. Check status
cd ~/workspaces/*/qa-workflow
./wsm status-v2

# 4. Create feature branch
./wsm branch-v2 create feature/qa-workflow-test

# 5. Make changes
echo "Workflow test change" >> test-repo-a/README.md
echo "func QATest() {}" >> test-go-repo/main.go

# 6. Check diff
./wsm diff-v2

# 7. Commit changes
./wsm commit-v2 --add-all -m "feat: Add QA workflow test changes"

# 8. Check status again
./wsm status-v2

# 9. Add another repository
./wsm add-v2 test-repo-c

# 10. Create tmux session
./wsm tmux-v2

# 11. Generate starship config
./wsm starship-v2

# 12. Get workspace info
./wsm info-v2

# 13. Fork workspace
./wsm fork-v2 qa-workflow-fork

# 14. Merge back
cd ~/workspaces/*/qa-workflow-fork
./wsm merge-v2 qa-workflow --dry-run

# 15. Clean up
./wsm delete-v2 qa-workflow-fork --force
```

### Scenario 2: Error Recovery

Test error handling and recovery:

```bash
# Test with corrupted workspace
cd ~/workspaces/*/qa-workflow
rm -f .workspace.json
./wsm status-v2

# Test with missing repository
./wsm remove-v2 test-repo-a --remove-files
./wsm status-v2

# Test recovery by re-adding
./wsm add-v2 test-repo-a
```

### Scenario 3: Performance Testing

Test with multiple repositories:

```bash
# Create many test repositories
mkdir -p ~/qa-perf-test
cd ~/qa-perf-test
for i in {1..10}; do
    mkdir repo-$i
    cd repo-$i
    git init
    echo "# Repo $i" > README.md
    git add .
    git commit -m "Initial commit"
    cd ..
done

# Test discovery performance
time ./wsm discover-v2 ~/qa-perf-test

# Test workspace creation performance
time ./wsm create-v2 qa-perf-workspace --repos repo-1,repo-2,repo-3,repo-4,repo-5

# Test status performance
cd ~/workspaces/*/qa-perf-workspace
time ./wsm status-v2
```

---

## Cross-Platform Testing

### Windows Testing (if applicable)

```powershell
# Test basic commands on Windows
.\wsm.exe discover-v2 C:\qa-test-repos
.\wsm.exe create-v2 qa-windows-test --repos test-repo-a
.\wsm.exe list-v2 workspaces
```

### macOS Testing (if applicable)

```bash
# Test with macOS-specific paths
./wsm discover-v2 ~/Desktop/qa-test-repos
./wsm create-v2 qa-macos-test --repos test-repo-a
```

---

## Backward Compatibility Testing

### Test V1 and V2 Coexistence

```bash
# Verify old commands still work
./wsm create qa-v1-test --repos test-repo-a
./wsm list workspaces

# Verify v2 commands work with v1-created workspaces
./wsm status-v2 qa-v1-test
./wsm info-v2 qa-v1-test

# Test mixed operations
./wsm add-v2 test-repo-b  # Add with v2
./wsm status              # Check with v1
./wsm remove-v2 test-repo-b  # Remove with v2
```

---

## Output Format Testing

### JSON Output Validation

Test all commands that support JSON output:

```bash
# Test JSON structure and validity
./wsm list-v2 workspaces --format json | jq '.'
./wsm status-v2 --format json | jq '.'
./wsm info-v2 --format json | jq '.'
./wsm commit-v2 --dry-run --format json -m "test" | jq '.'
./wsm push-v2 --dry-run --format json | jq '.'
./wsm sync-v2 --format json | jq '.'
```

### Table Output Validation

Verify table formatting is consistent and readable:

```bash
./wsm list-v2 repos
./wsm list-v2 workspaces
./wsm status-v2
./wsm branch-v2 list
```

---

## Error Handling Testing

### Network Errors

```bash
# Test with network disconnected (if possible)
# Disable network connection
./wsm sync-v2  # Should handle network errors gracefully
./wsm push-v2  # Should show clear error messages
```

### Permission Errors

```bash
# Test with permission issues
chmod 000 ~/.config/workspace-manager
./wsm list-v2 workspaces  # Should show permission error

# Restore permissions
chmod 755 ~/.config/workspace-manager
```

---

## Performance Benchmarks

### Command Execution Times

Record baseline performance for comparison:

```bash
# Measure key command performance
echo "=== Performance Benchmarks ===" > qa-performance-results.txt

echo "discover-v2:" >> qa-performance-results.txt
time ./wsm discover-v2 ~/qa-test-repos 2>&1 | tail -3 >> qa-performance-results.txt

echo "create-v2:" >> qa-performance-results.txt
time ./wsm create-v2 qa-perf-test --repos test-repo-a,test-repo-b 2>&1 | tail -3 >> qa-performance-results.txt

echo "status-v2:" >> qa-performance-results.txt
cd ~/workspaces/*/qa-perf-test
time ./wsm status-v2 2>&1 | tail -3 >> qa-performance-results.txt

echo "list-v2:" >> qa-performance-results.txt
time ./wsm list-v2 workspaces 2>&1 | tail -3 >> qa-performance-results.txt

cat qa-performance-results.txt
```

### Memory Usage

```bash
# Monitor memory usage during operations
./wsm create-v2 qa-memory-test --repos test-repo-a,test-repo-b,test-repo-c &
PID=$!
while kill -0 $PID 2>/dev/null; do
    ps -p $PID -o pid,rss,vsz,comm
    sleep 1
done
```

---

## Security Testing

### Input Validation

```bash
# Test with malicious input
./wsm create-v2 "../../../malicious" --repos test-repo-a
./wsm create-v2 "workspace; rm -rf /" --repos test-repo-a
./wsm add-v2 "repo && malicious-command"

# All should be safely handled without executing malicious code
```

### File Permission Validation

```bash
# Check created files have correct permissions
./wsm create-v2 qa-security-test --repos test-repo-a
ls -la ~/workspaces/*/qa-security-test/
ls -la ~/.config/workspace-manager/workspaces/
```

---

## Cleanup

### Post-Testing Cleanup

```bash
# Clean up test workspaces
./wsm delete-v2 qa-test-workspace --force --remove-files 2>/dev/null || true
./wsm delete-v2 qa-workflow --force --remove-files 2>/dev/null || true
./wsm delete-v2 qa-perf-workspace --force --remove-files 2>/dev/null || true
./wsm delete-v2 qa-security-test --force --remove-files 2>/dev/null || true

# Clean up test repositories
rm -rf ~/qa-test-repos
rm -rf ~/qa-perf-test

# Restore original configuration
rm -rf ~/.config/workspace-manager
mv ~/.config/workspace-manager.backup ~/.config/workspace-manager 2>/dev/null || true

# Remove performance test files
rm -f qa-performance-results.txt
```

---

## Test Results Documentation

### Expected Results Template

For each test, document:

1. **Command Tested**: `./wsm command-v2 args`
2. **Expected Behavior**: What should happen
3. **Actual Result**: What actually happened
4. **Status**: ✅ Pass / ❌ Fail / ⚠️ Partial
5. **Notes**: Any observations or issues

### Critical Success Criteria

The following MUST work for the migration to be considered successful:

- ✅ All 15 v2 commands build and execute without errors
- ✅ Help system works for all commands
- ✅ Service architecture properly abstracts external dependencies
- ✅ Error handling provides clear, actionable messages
- ✅ JSON output is valid and consistent across commands
- ✅ Backward compatibility with v1-created workspaces
- ✅ File operations respect filesystem permissions and paths
- ✅ Git operations work correctly with worktrees
- ✅ Performance is acceptable for normal use cases

### Known Limitations

Document any known limitations or issues:

- Tmux integration requires tmux to be installed
- Push operations require configured git remotes
- Some operations require internet connectivity
- Interactive modes require terminal input capabilities

---

**QA Testing Checklist Complete**

This comprehensive testing protocol validates the entire WSM v2 migration across all commands, integration scenarios, error cases, and performance benchmarks. Following this checklist ensures the service architecture migration is production-ready.
