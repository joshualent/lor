package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "lor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lor.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db.Close()
}

func TestGrantConditionalUpdate(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	a := &Agent{Name: "claude", Status: "active", LANAddrs: []string{"192.168.1.50"}, CreatedAt: 1}
	if err := db.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	h := &Host{Name: "nas", Address: "192.168.1.20", MaxLevel: "sudo", LorInfra: true, CreatedAt: 1}
	if err := db.CreateHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetHostByName(ctx, "nas")
	if err != nil || !got.LorInfra || got.Hypervisor {
		t.Fatalf("host = %+v err=%v", got, err)
	}
	g := &Grant{AgentID: a.ID, HostID: h.ID, Origin: "admin_offer", Level: "sudo", RequestedLevel: "sudo",
		RequestedDurationS: 900, Status: StatusOffered, RequestedAt: 10, ValidAfter: 10, ValidBefore: 910}
	if err := db.CreateGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.Status = StatusActive
	g.PublicKey = "ssh-ed25519 AAAA"
	if err := db.UpdateGrant(ctx, g, StatusOffered); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := db.UpdateGrant(ctx, g, StatusOffered); !errors.Is(err, ErrConflict) {
		t.Fatalf("second claim should conflict, got %v", err)
	}
	list, err := db.ListGrants(ctx, GrantFilter{AgentID: a.ID, Statuses: []string{StatusActive, StatusPending}})
	if err != nil || len(list) != 1 || list[0].PublicKey != "ssh-ed25519 AAAA" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
}

func TestAuditAppendOnly(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	e := &AuditEvent{TS: 1, Actor: "system", Action: "ca.generated", Details: map[string]any{"fp": "SHA256:x"}}
	if err := db.InsertAudit(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE audit_events SET actor = 'x'`); err == nil {
		t.Fatal("update must fail")
	}
	if _, err := db.db.ExecContext(ctx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("delete must fail")
	}
	list, err := db.ListAudit(ctx, AuditFilter{Action: "ca."})
	if err != nil || len(list) != 1 || list[0].Details["fp"] != "SHA256:x" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	list, _ = db.ListAudit(ctx, AuditFilter{Action: "c_%"})
	if len(list) != 0 {
		t.Fatal("LIKE wildcards must be escaped")
	}
}

func TestEnrollmentSingleUse(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	a := &Agent{Name: "claude", Status: "active", LANAddrs: []string{"192.168.1.50"}, EnrollCodeHash: []byte("code"), EnrollCodeExpiresAt: 100, CreatedAt: 1}
	if err := db.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteEnrollment(ctx, a.ID, []byte("code"), []byte("tok"), 101); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired code: %v", err)
	}
	if err := db.CompleteEnrollment(ctx, a.ID, []byte("code"), []byte("tok"), 50); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if err := db.CompleteEnrollment(ctx, a.ID, []byte("code"), []byte("tok2"), 50); !errors.Is(err, ErrConflict) {
		t.Fatalf("reuse: %v", err)
	}
	got, err := db.GetAgentByTokenHash(ctx, []byte("tok"))
	if err != nil || got.ID != a.ID {
		t.Fatalf("by token: %v", err)
	}
}

func TestSetupCodeAndReset(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	if err := db.CreateSetupCode(ctx, []byte("h"), 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := db.ConsumeSetupCode(ctx, []byte("h"), 50); err != nil {
		t.Fatal(err)
	}
	if err := db.ConsumeSetupCode(ctx, []byte("h"), 50); !errors.Is(err, ErrConflict) {
		t.Fatalf("second use: %v", err)
	}
	p := &Passkey{Name: "iPhone", CredentialID: []byte{1}, PublicKey: []byte{2}, CredentialJSON: "{}", CreatedAt: 1}
	if err := db.CreatePasskey(ctx, p); err != nil {
		t.Fatal(err)
	}
	s := &Session{TokenHash: []byte("s"), PasskeyID: p.ID, CreatedAt: 1, LastSeenAt: 1, ExpiresAt: 100}
	if err := db.CreateSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountPasskeys(ctx); n != 0 {
		t.Fatal("passkeys remain")
	}
	if ss, _ := db.ListSessions(ctx); len(ss) != 0 {
		t.Fatal("sessions remain")
	}
}
