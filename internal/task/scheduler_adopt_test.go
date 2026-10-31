package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdoptOrReclaimContainer_RunningIsAdopted is a regression test for
// issue #57: a container left running from an earlier, untracked `devx
// up` (e.g. devx was killed before it could record state) must be
// adopted as-is — not restarted, and not removed — so a fresh `docker
// run --name` for the same task doesn't collide with it.
func TestAdoptOrReclaimContainer_RunningIsAdopted(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withStdout("running"), withArgsFile(argsFile))
	withFakePath(t, dir)

	s := &Scheduler{}
	et := newEffectiveTask("api", "docker run -d --name api my-image", ".")
	et.ContainerName = "api"

	adopted, err := s.adoptOrReclaimContainer(context.Background(), et)
	if err != nil {
		t.Fatalf("adoptOrReclaimContainer: %v", err)
	}
	if !adopted {
		t.Fatal("expected a running container to be adopted")
	}

	args, rerr := os.ReadFile(argsFile)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if containsField(string(args), "rm") {
		t.Errorf("a running, adopted container must not be removed; docker was called with: %s", args)
	}
}

// containsField reports whether s contains tok as a whole whitespace-
// separated field (unlike strings.Contains, which would false-positive
// on e.g. "--format" containing "rm").
func containsField(s, tok string) bool {
	for _, f := range strings.Fields(s) {
		if f == tok {
			return true
		}
	}
	return false
}

// TestAdoptOrReclaimContainer_StoppedIsReclaimed confirms a stopped (or
// never-existing — Status() reports both as "exited") container is
// removed via `rm -f` to clear the name, so the upcoming `docker run
// --name` doesn't fail with "name already in use".
func TestAdoptOrReclaimContainer_StoppedIsReclaimed(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	dir := fakeRuntimeBin(t, "docker", withStdout("exited"), withArgsFile(argsFile))
	withFakePath(t, dir)

	s := &Scheduler{}
	et := newEffectiveTask("api", "docker run -d --name api my-image", ".")
	et.ContainerName = "api"

	adopted, err := s.adoptOrReclaimContainer(context.Background(), et)
	if err != nil {
		t.Fatalf("adoptOrReclaimContainer: %v", err)
	}
	if adopted {
		t.Fatal("expected a stopped container not to be adopted")
	}

	args, rerr := os.ReadFile(argsFile)
	if rerr != nil {
		t.Fatal(rerr)
	}
	argList := strings.Fields(string(args))
	if !containsField(string(args), "rm") || !containsField(string(args), "-f") || argList[len(argList)-1] != "api" {
		t.Errorf("expected a final `rm -f api` call to clear the name, docker was called with: %s", args)
	}
}
