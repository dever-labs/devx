package main

import (
    "bufio"
    "context"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"

    "github.com/dever-labs/devx/internal/compose"
    "github.com/dever-labs/devx/internal/config"
    "github.com/dever-labs/devx/internal/doctor"
    "github.com/dever-labs/devx/internal/graph"
    "github.com/dever-labs/devx/internal/k8s"
    "github.com/dever-labs/devx/internal/lock"
    "github.com/dever-labs/devx/internal/runtime"
    "github.com/dever-labs/devx/internal/ui"
)

const (
    manifestFile = "devx.yaml"
    devxDir      = ".devx"
    composeFile  = "compose.yaml"
    k8sFile      = "k8s.yaml"
    stateFile    = "state.json"
    lockFile     = "devx.lock"
)

type state struct {
    Profile string `json:"profile"`
    Runtime string `json:"runtime"`
    Telemetry bool `json:"telemetry"`
}

func main() {
    if len(os.Args) < 2 {
        printUsage()
        os.Exit(1)
    }

    ctx := context.Background()
    cmd := os.Args[1]
    args := os.Args[2:]

    var err error
    switch cmd {
    case "init":
        err = runInit(args)
    case "up":
        err = runUp(ctx, args)
    case "down":
        err = runDown(ctx, args)
    case "status":
        err = runStatus(ctx, args)
    case "logs":
        err = runLogs(ctx, args)
    case "exec":
        err = runExec(ctx, args)
    case "doctor":
        err = runDoctor(ctx, args)
    case "render":
        err = runRender(ctx, args)
    case "lock":
        err = runLock(ctx, args)
    case "help", "-h", "--help":
        printUsage()
        return
    default:
        fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
        printUsage()
        os.Exit(1)
    }

    if err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
}

func printUsage() {
    fmt.Println("devx - cross-platform dev orchestrator")
    fmt.Println("\nUsage:")
    fmt.Println("  devx init")
    fmt.Println("  devx up [--profile local|ci|k8s] [--build] [--pull] [--no-telemetry]")
    fmt.Println("  devx down [--volumes]")
    fmt.Println("  devx status")
    fmt.Println("  devx logs [service] [--follow] [--since 10m] [--json]")
    fmt.Println("  devx exec <service> -- <cmd...>")
    fmt.Println("  devx doctor [--fix]")
    fmt.Println("  devx render compose [--write] [--no-telemetry]")
    fmt.Println("  devx render k8s [--profile name] [--namespace ns] [--write]")
    fmt.Println("  devx lock update")
}

func runInit(args []string) error {
    fs := flag.NewFlagSet("init", flag.ExitOnError)
    _ = fs.Parse(args)

    if fileExists(manifestFile) {
        return fmt.Errorf("%s already exists", manifestFile)
    }

    stub := "version: 1\n\nproject:\n  name: my-app\n  defaultProfile: local\n\nprofiles:\n  local:\n    services:\n      api:\n        build:\n          context: ./api\n          dockerfile: Dockerfile\n        ports:\n          - \"8080:8080\"\n        env:\n          ASPNETCORE_ENVIRONMENT: Development\n        dependsOn: [db]\n        health:\n          httpGet: \"http://localhost:8080/health\"\n          interval: 5s\n          retries: 30\n\n    deps:\n      db:\n        kind: postgres\n        version: \"16\"\n        env:\n          POSTGRES_PASSWORD: postgres\n        ports: [\"5432:5432\"]\n        volume: \"db-data:/var/lib/postgresql/data\"\n"

    if err := os.WriteFile(manifestFile, []byte(stub), 0644); err != nil {
        return err
    }

    if err := ensureDevxDir(); err != nil {
        return err
    }

    if err := ensureGitignore(); err != nil {
        return err
    }

    fmt.Println("Initialized devx.yaml")
    return nil
}

func runUp(ctx context.Context, args []string) error {
    fs := flag.NewFlagSet("up", flag.ExitOnError)
    profile := fs.String("profile", "", "Profile to use")
    build := fs.Bool("build", false, "Build images")
    pull := fs.Bool("pull", false, "Always pull images")
    noTelemetry := fs.Bool("no-telemetry", false, "Disable telemetry stack")
    _ = fs.Parse(args)

    manifest, profName, prof, err := loadProfile(*profile)
    if err != nil {
        return err
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    lockfile, _ := lock.Load(lockFile)

    if err := ensureDevxDir(); err != nil {
        return err
    }

    runtimeMode := profileRuntime(prof)
    if runtimeMode == "k8s" {
        return runUpK8s(ctx, manifest, profName, prof)
    }

    composePath := filepath.Join(devxDir, composeFile)
    enableTelemetry := !*noTelemetry
    if err := writeCompose(composePath, manifest, profName, prof, lockfile, enableTelemetry); err != nil {
        return err
    }

    if err := rt.Up(ctx, composePath, manifest.Project.Name, runtime.UpOptions{Build: *build, Pull: *pull}); err != nil {
        return err
    }

    if err := waitForHealth(prof); err != nil {
        return err
    }

    _ = writeState(state{Profile: profName, Runtime: rt.Name(), Telemetry: enableTelemetry})

    fmt.Println("Environment is up")
    return nil
}

func runDown(ctx context.Context, args []string) error {
    fs := flag.NewFlagSet("down", flag.ExitOnError)
    volumes := fs.Bool("volumes", false, "Remove volumes")
    _ = fs.Parse(args)

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    enableTelemetry := telemetryFromState()
    runtimeMode := profileRuntime(prof)
    if runtimeMode == "k8s" {
        return runDownK8s(ctx)
    }

    composePath := filepath.Join(devxDir, composeFile)
    if !fileExists(composePath) {
        if err := ensureDevxDir(); err != nil {
            return err
        }
        if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
            return err
        }
    }

    return rt.Down(ctx, composePath, manifest.Project.Name, *volumes)
}

func runStatus(ctx context.Context, args []string) error {
    fs := flag.NewFlagSet("status", flag.ExitOnError)
    _ = fs.Parse(args)

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    if profileRuntime(prof) == "k8s" {
        return errors.New("status for k8s runtime is not supported yet")
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    enableTelemetry := telemetryFromState()
    composePath := filepath.Join(devxDir, composeFile)
    if err := ensureDevxDir(); err != nil {
        return err
    }
    if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
        return err
    }

    statuses, err := rt.Status(ctx, composePath, manifest.Project.Name)
    if err != nil {
        return err
    }

    sort.Slice(statuses, func(i, j int) bool {
        return statuses[i].Name < statuses[j].Name
    })

    headers := []string{"Service", "State", "Health", "Ports"}
    rows := make([][]string, 0, len(statuses))
    for _, st := range statuses {
        rows = append(rows, []string{st.Name, st.State, st.Health, st.Ports})
    }
    ui.PrintTable(os.Stdout, headers, rows)
    return nil
}

func runLogs(ctx context.Context, args []string) error {
    fs := flag.NewFlagSet("logs", flag.ExitOnError)
    follow := fs.Bool("follow", false, "Follow logs")
    since := fs.String("since", "", "Show logs since")
    jsonOut := fs.Bool("json", false, "JSON output")
    _ = fs.Parse(args)

    var service string
    if fs.NArg() > 0 {
        service = fs.Arg(0)
    }

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    if profileRuntime(prof) == "k8s" {
        return errors.New("logs for k8s runtime are not supported yet")
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    enableTelemetry := telemetryFromState()
    composePath := filepath.Join(devxDir, composeFile)
    if err := ensureDevxDir(); err != nil {
        return err
    }
    if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
        return err
    }

    reader, err := rt.Logs(ctx, composePath, manifest.Project.Name, runtime.LogsOptions{
        Service: service,
        Follow:  *follow,
        Since:   *since,
        JSON:    *jsonOut,
    })
    if err != nil {
        return err
    }
    defer reader.Close()

    return streamLogs(reader, *jsonOut)
}

func runExec(ctx context.Context, args []string) error {
    if len(args) == 0 {
        return errors.New("exec requires a service name")
    }

    sep := -1
    for i, arg := range args {
        if arg == "--" {
            sep = i
            break
        }
    }
    if sep == -1 || sep == len(args)-1 {
        return errors.New("exec requires a command after --")
    }

    service := args[0]
    cmdArgs := args[sep+1:]

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    if profileRuntime(prof) == "k8s" {
        return errors.New("exec for k8s runtime is not supported yet")
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    enableTelemetry := telemetryFromState()
    composePath := filepath.Join(devxDir, composeFile)
    if err := ensureDevxDir(); err != nil {
        return err
    }
    if err := writeCompose(composePath, manifest, profName, prof, nil, enableTelemetry); err != nil {
        return err
    }

    code, err := rt.Exec(ctx, composePath, manifest.Project.Name, service, cmdArgs)
    if err != nil {
        return err
    }
    if code != 0 {
        return fmt.Errorf("exec exited with code %d", code)
    }
    return nil
}

func runDoctor(ctx context.Context, args []string) error {
    fs := flag.NewFlagSet("doctor", flag.ExitOnError)
    fix := fs.Bool("fix", false, "Attempt fixes")
    _ = fs.Parse(args)

    manifest, _, _, _ := loadProfile("")

    report := doctor.Run(ctx, doctor.Options{
        Manifest: manifest,
        Fix:      *fix,
    })

    doctor.PrintReport(os.Stdout, report)
    if report.HasFailures() {
        return errors.New("doctor found failures")
    }
    return nil
}

func runUpK8s(ctx context.Context, manifest *config.Manifest, profName string, prof *config.Profile) error {
    output, err := k8s.Render(manifest, profName, prof, "")
    if err != nil {
        return err
    }

    if err := ensureDevxDir(); err != nil {
        return err
    }
    path := filepath.Join(devxDir, k8sFile)
    if err := os.WriteFile(path, []byte(output), 0644); err != nil {
        return err
    }

    if err := k8s.Apply(ctx, path); err != nil {
        return err
    }

    _ = writeState(state{Profile: profName, Runtime: "k8s", Telemetry: false})
    fmt.Println("Kubernetes resources applied")
    return nil
}

func runDownK8s(ctx context.Context) error {
    path := filepath.Join(devxDir, k8sFile)
    if !fileExists(path) {
        return fmt.Errorf("%s not found; run 'devx render k8s --write' or 'devx up --profile <k8s profile>'", path)
    }
    if err := k8s.Delete(ctx, path); err != nil {
        return err
    }
    fmt.Println("Kubernetes resources deleted")
    return nil
}

func runRender(ctx context.Context, args []string) error {
    if len(args) == 0 || args[0] != "compose" {
        return runRenderK8s(ctx, args)
    }

    fs := flag.NewFlagSet("render", flag.ExitOnError)
    write := fs.Bool("write", false, "Write to .devx/compose.yaml")
    noTelemetry := fs.Bool("no-telemetry", false, "Disable telemetry stack")
    _ = fs.Parse(args[1:])

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    lockfile, _ := lock.Load(lockFile)

    composed, err := buildCompose(manifest, profName, prof, lockfile, !*noTelemetry)
    if err != nil {
        return err
    }

    if *write {
        if err := ensureDevxDir(); err != nil {
            return err
        }
        composePath := filepath.Join(devxDir, composeFile)
        return writeCompose(composePath, manifest, profName, prof, lockfile, !*noTelemetry)
    }

    fmt.Print(composed)
    return nil
}

func runRenderK8s(ctx context.Context, args []string) error {
    if len(args) == 0 || args[0] != "k8s" {
        return errors.New("render requires 'compose' or 'k8s'")
    }

    fs := flag.NewFlagSet("render-k8s", flag.ExitOnError)
    profile := fs.String("profile", "", "Profile to use")
    namespace := fs.String("namespace", "", "Kubernetes namespace")
    write := fs.Bool("write", false, "Write to .devx/k8s.yaml")
    _ = fs.Parse(args[1:])

    manifest, profName, prof, err := loadProfile(*profile)
    if err != nil {
        return err
    }

    output, err := k8s.Render(manifest, profName, prof, *namespace)
    if err != nil {
        return err
    }

    if *write {
        if err := ensureDevxDir(); err != nil {
            return err
        }
        return os.WriteFile(filepath.Join(devxDir, k8sFile), []byte(output), 0644)
    }

    fmt.Print(output)
    return nil
}

func runLock(ctx context.Context, args []string) error {
    if len(args) == 0 || args[0] != "update" {
        return errors.New("lock requires 'update'")
    }

    manifest, profName, prof, err := loadProfile("")
    if err != nil {
        return err
    }

    rt, err := runtime.SelectRuntime(ctx)
    if err != nil {
        return err
    }

    resolver, ok := rt.(runtime.DigestResolver)
    if !ok {
        return errors.New("runtime does not support digest resolution")
    }

    lf := lock.New()
    images, err := collectImages(manifest, profName, prof)
    if err != nil {
        return err
    }

    for _, image := range images {
        digest, err := resolver.ResolveImageDigest(ctx, image)
        if err != nil {
            return fmt.Errorf("lock update failed for %s: %w", image, err)
        }
        lf.Images[image] = digest
    }

    return lock.Save(lockFile, lf)
}

func loadProfile(profile string) (*config.Manifest, string, *config.Profile, error) {
    manifest, err := config.Load(manifestFile)
    if err != nil {
        return nil, "", nil, err
    }

    if err := config.Validate(manifest); err != nil {
        return nil, "", nil, err
    }

    profName := profile
    if profName == "" {
        profName = manifest.Project.DefaultProfile
    }
    if profName == "" {
        return nil, "", nil, errors.New("no profile specified and no defaultProfile set in manifest")
    }

    prof, err := config.ProfileByName(manifest, profName)
    if err != nil {
        return nil, "", nil, err
    }

    if err := config.ValidateProfile(manifest, profName); err != nil {
        return nil, "", nil, err
    }

    return manifest, profName, prof, nil
}

func writeCompose(path string, manifest *config.Manifest, profName string, prof *config.Profile, lockfile *lock.Lockfile, enableTelemetry bool) error {
    composed, err := buildCompose(manifest, profName, prof, lockfile, enableTelemetry)
    if err != nil {
        return err
    }
    if err := os.WriteFile(path, []byte(composed), 0644); err != nil {
        return err
    }

    assets := compose.TelemetryAssets(enableTelemetry)
    if len(assets) == 0 {
        return nil
    }

    baseDir := filepath.Dir(path)
    for _, asset := range assets {
        assetPath := filepath.Join(baseDir, asset.Path)
        if err := os.MkdirAll(filepath.Dir(assetPath), 0755); err != nil {
            return err
        }
        if err := os.WriteFile(assetPath, asset.Content, 0644); err != nil {
            return err
        }
    }

    return nil
}

func buildCompose(manifest *config.Manifest, profName string, prof *config.Profile, lockfile *lock.Lockfile, enableTelemetry bool) (string, error) {
    g, err := graph.Build(prof)
    if err != nil {
        return "", err
    }
    if _, err := graph.TopoSort(g); err != nil {
        return "", err
    }

    rewrite := compose.RewriteOptions{
        RegistryPrefix: manifest.Registry.Prefix,
        Lockfile:       lockfile,
    }

    return compose.Render(manifest, profName, prof, rewrite, enableTelemetry)
}

func profileRuntime(prof *config.Profile) string {
    if prof == nil || prof.Runtime == "" {
        return "compose"
    }
    return prof.Runtime
}

func ensureDevxDir() error {
    return os.MkdirAll(devxDir, 0755)
}

func ensureGitignore() error {
    path := ".gitignore"
    entry := ".devx/"

    if !fileExists(path) {
        return os.WriteFile(path, []byte(entry+"\n"), 0644)
    }

    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }

    if strings.Contains(string(data), entry) {
        return nil
    }

    f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.WriteString("\n" + entry + "\n")
    return err
}

func fileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}

func writeState(s state) error {
    if err := ensureDevxDir(); err != nil {
        return err
    }
    data, err := json.MarshalIndent(s, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filepath.Join(devxDir, stateFile), data, 0644)
}

func readState() *state {
    data, err := os.ReadFile(filepath.Join(devxDir, stateFile))
    if err != nil {
        return nil
    }
    var s state
    if err := json.Unmarshal(data, &s); err != nil {
        return nil
    }
    return &s
}

func telemetryFromState() bool {
    st := readState()
    if st == nil {
        return true
    }
    return st.Telemetry
}

func streamLogs(reader io.Reader, jsonOut bool) error {
    scanner := bufio.NewScanner(reader)
    for scanner.Scan() {
        line := scanner.Text()
        if !jsonOut {
            fmt.Println(line)
            continue
        }
        entry := map[string]string{
            "line": line,
        }
        data, _ := json.Marshal(entry)
        fmt.Println(string(data))
    }
    return scanner.Err()
}

func waitForHealth(profile *config.Profile) error {
    if profile == nil {
        return nil
    }

    type check struct {
        name string
        url  string
    }

    var checks []check
    for name, svc := range profile.Services {
        if svc.Health == nil || svc.Health.HttpGet == "" {
            continue
        }
        checks = append(checks, check{name: name, url: svc.Health.HttpGet})
    }

    if len(checks) == 0 {
        return nil
    }

    deadline := time.Now().Add(2 * time.Minute)
    pending := map[string]string{}
    for _, c := range checks {
        pending[c.name] = c.url
    }

    for len(pending) > 0 && time.Now().Before(deadline) {
        for name, url := range pending {
            if checkHTTP(url) {
                delete(pending, name)
            }
        }
        if len(pending) == 0 {
            break
        }
        time.Sleep(2 * time.Second)
    }

    if len(pending) > 0 {
        var parts []string
        for name, url := range pending {
            parts = append(parts, fmt.Sprintf("%s (%s)", name, url))
        }
        return fmt.Errorf("health checks failed: %s", strings.Join(parts, ", "))
    }

    return nil
}

func checkHTTP(url string) bool {
    client := http.Client{Timeout: 2 * time.Second}
    resp, err := client.Get(url)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func collectImages(manifest *config.Manifest, profileName string, prof *config.Profile) ([]string, error) {
    composed, err := buildCompose(manifest, profileName, prof, nil, true)
    if err != nil {
        return nil, err
    }

    imgs, err := compose.CollectImages([]byte(composed))
    if err != nil {
        return nil, err
    }

    return imgs, nil
}
