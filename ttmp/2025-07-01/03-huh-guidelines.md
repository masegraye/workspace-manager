# Huh Interactive Forms Guidelines

## Core Pattern
```go
form := huh.NewForm(
    huh.NewGroup(
        // Add form elements here
    ),
)
err := form.Run()
```

## Form Types

### Confirmation Dialog
```go
var confirmed bool
huh.NewConfirm().
    Title("Are you sure?").
    Description("Optional explanation").
    Value(&confirmed)
```

### Single Select
```go
var choice string
huh.NewSelect[string]().
    Title("Choose an option:").
    Options(
        huh.NewOption("Label 1", "value1"),
        huh.NewOption("Label 2", "value2"),
    ).
    Value(&choice)
```

### Multi-Select
```go
var selected []string
huh.NewMultiSelect[string]().
    Title("Choose multiple:").
    Options(options...).  // []huh.Option[string]
    Value(&selected)
```

### Text Input
```go
var input string
huh.NewInput().
    Title("Enter value:").
    Placeholder("Type here...").
    Value(&input)
```

## Error Handling Pattern
```go
err := form.Run()
if err != nil {
    errMsg := strings.ToLower(err.Error())
    if strings.Contains(errMsg, "user aborted") ||
       strings.Contains(errMsg, "cancelled") ||
       strings.Contains(errMsg, "interrupt") {
        return errors.New("operation cancelled by user")
    }
    return errors.Wrap(err, "form error")
}
```

## Best Practices

1. **Always handle cancellation** - Users expect Ctrl+C to work
2. **Use descriptive titles** - Make the choice clear
3. **Add descriptions for destructive actions** - "This cannot be undone"
4. **Create dynamic options** from data structures
5. **Use typed selects** - `huh.NewSelect[string]()` for type safety
6. **Group related prompts** in single forms when logical
7. **Provide escape hatches** - Always offer cancel/abort options

## Quick Examples

**Yes/No with context:**
```go
huh.NewConfirm().
    Title("Delete workspace 'my-project'?").
    Description("This action cannot be undone.").
    Value(&confirmed)
```

**Dynamic multi-select:**
```go
var options []huh.Option[string]
for _, repo := range repos {
    label := fmt.Sprintf("%s (%s)", repo.Name, repo.Category)
    options = append(options, huh.NewOption(label, repo.Name))
}
```
