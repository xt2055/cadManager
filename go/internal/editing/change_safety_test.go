package editing

import (
	"context"
	"os"
	"testing"
)

func TestAdminCaptureFailurePreservesWorkingSession(t *testing.T) {
	env := newTestEnv(t)
	env.service.SetChangeGate(&fakeChangeGate{executing: true})
	key := "working/admin-failure.dwg"
	session := setupWorkingSession(t, env, key, "unsaved-work")
	env.versions.failCapture = true
	if _, err := env.service.Close(context.Background(), adminUser, session.ID); err == nil {
		t.Fatal("capture failure must not report success")
	}
	if sessionStatus(t, env, session.ID) != "active" {
		t.Fatal("session was closed")
	}
	if _, err := os.Stat(workFilePath(t, env, key)); err != nil {
		t.Fatal(err)
	}
}

func TestWorkingObjectFailureDoesNotFallBack(t *testing.T) {
	env := newTestEnv(t)
	env.service.SetChangeGate(&fakeChangeGate{workKey: "missing/work.dwg"})
	if _, err := env.service.resolveWorkKey(context.Background(), designerUser, fixtureAttachment(), "req-1"); err == nil {
		t.Fatal("missing work object must fail instead of falling back")
	}
}
