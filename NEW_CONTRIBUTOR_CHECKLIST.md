# New Contributor Quick Start Checklist

Welcome! Here's your step-by-step checklist to start contributing to the WSM migration:

## ✅ Getting Started (Day 1)

- [ ] **Read the Migration Guide**: Read `MIGRATION_GUIDE.md` thoroughly
- [ ] **Understand the Architecture**: Study the new vs old architecture diagrams
- [ ] **Build the Project**: `go build ./cmd/wsm` should work
- [ ] **Test V2 Commands**: Try `./wsm create-v2 --help`, `./wsm discover-v2 --help`, etc.
- [ ] **Study the Pattern**: Read `cmd/cmds/cmd_create_v2.go` to understand the v2 pattern

## ✅ Understanding the Codebase (Day 1-2)

- [ ] **Explore New Architecture**: Look at `pkg/wsm/service/workspace.go` (287 lines, clean!)
- [ ] **Compare with Old**: Look at `pkg/wsm/workspace.go` (1,862 lines, messy!)
- [ ] **Check Domain Models**: Look at `pkg/wsm/domain/types.go`
- [ ] **Understand Services**: Browse `pkg/wsm/worktree/`, `pkg/wsm/discovery/`, etc.
- [ ] **Run Tests**: `go test ./pkg/wsm/service/...` to see the tests work

## ✅ Pick Your First Command (Day 2)

Choose one of these high-priority commands to migrate:

**Beginner Friendly:**
- [ ] `list` - List existing workspaces (simple, high usage)
- [ ] `info` - Show workspace information (read-only, safe)

**Intermediate:**
- [ ] `delete` - Delete workspaces (has prompting, cleanup logic)
- [ ] `add` - Add repositories to workspace (modifies state)

**Advanced:**
- [ ] `tmux` - Create tmux sessions (complex integration)

## ✅ Migration Process (Day 2-3)

- [ ] **Copy Template**: `cp cmd/cmds/cmd_create_v2.go cmd/cmds/cmd_[YOUR_COMMAND]_v2.go`
- [ ] **Study Old Command**: Look at the existing `cmd_[YOUR_COMMAND].go` to understand what it does
- [ ] **Follow the Pattern**: Replace the logic with service calls following the v2 pattern
- [ ] **Add Service Methods**: If needed, add methods to `pkg/wsm/service/workspace.go`
- [ ] **Test Thoroughly**: Create test workspaces and verify your command works
- [ ] **Handle Edge Cases**: Test error conditions, user cancellation, etc.

## ✅ Code Quality (Throughout)

- [ ] **Follow Go Conventions**: Use proper error handling, naming, etc.
- [ ] **Use Dependency Injection**: Always use `service.NewDeps()` and `service.NewWorkspaceService(deps)`
- [ ] **Proper Logging**: Use `deps.Logger.Info()` with `ux.Field()` for structured logging
- [ ] **Error Handling**: Use `errors.Wrap()` for context, handle user cancellation gracefully
- [ ] **No Direct Dependencies**: Never call `exec.Command()`, `os.*`, or `fmt.Printf()` directly

## ✅ Testing (Day 3)

- [ ] **Unit Tests**: Write tests for any new service methods
- [ ] **Integration Tests**: Test the full command workflow
- [ ] **Manual Testing**: Create real workspaces and test edge cases
- [ ] **Regression Testing**: Ensure old behavior is preserved

## ✅ Submission (Day 3-4)

- [ ] **Run All Tests**: `go test ./...` should pass
- [ ] **Run Linting**: `golangci-lint run -v` should pass
- [ ] **Build Successfully**: `go build ./...` should work
- [ ] **Register Command**: Add your command to `cmd/wsm/root.go`
- [ ] **Write Clear PR**: Describe what you migrated and how to test it

## 📚 Resources

- **Architecture Reference**: `pkg/wsm/service/workspace.go`
- **CLI Pattern**: `cmd/cmds/cmd_create_v2.go`
- **Testing Pattern**: `pkg/wsm/service/workspace_test.go`
- **Domain Models**: `pkg/wsm/domain/types.go`
- **Migration Guide**: `MIGRATION_GUIDE.md`

## 🆘 Getting Help

**Common Questions:**
- **"Service doesn't have method I need"**: Add it following the pattern, use dependency injection
- **"Old command has complex prompting"**: Use `deps.Prompter` interface, check create-v2 for examples
- **"Not sure about error handling"**: Always use `errors.Wrap()`, handle cancellation separately
- **"Tests failing"**: Check interfaces are mocked, no direct filesystem dependencies

**When Stuck:**
- Look at the 4 working v2 commands for patterns
- Check the service implementations for examples
- Remember: every command follows the same pattern!

## 🎯 Success Criteria

Your migration is successful when:
- [ ] ✅ New v2 command works exactly like the old command
- [ ] ✅ All tests pass
- [ ] ✅ No breaking changes to existing functionality
- [ ] ✅ Code follows the established v2 pattern
- [ ] ✅ Proper error handling and user experience
- [ ] ✅ Good test coverage for any new service methods

## 🎉 What You're Achieving

Every command you migrate:
- ✅ Makes WSM more maintainable
- ✅ Reduces the 1,862-line god file
- ✅ Adds test coverage
- ✅ Improves developer experience
- ✅ Brings us closer to deleting the god file entirely!

Welcome to the team! 🚀
