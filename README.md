# CLI

A personal command-line toolkit

## Testing with `go run`

If you want to test a command without compiling, use:

```bash
go run main.go [command]
```

## Compilation

Execute in the project root (where main.go is located) to generate the executable:

```bash
go build -o mycli
```

Then run the commands with:

```bash
./mycli [command]
```
