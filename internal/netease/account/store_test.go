package account

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", "accounts.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func saveTestAccount(t *testing.T, s *Store, owner string, user int64, cookie string) Record {
	t.Helper()
	r, err := s.Save(owner, user, cookie)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assertRecord(t *testing.T, s *Store, want Record) {
	t.Helper()
	got, err := s.Get(want.Owner, want.ID)
	if err != nil || got != want {
		t.Fatalf("snapshot mismatch: error=%v", err)
	}
}

func TestRestartPersistenceAndOwnerIsolation(t *testing.T) {
	s, path := openTestStore(t)
	a := saveTestAccount(t, s, "caller:alice", 10, "alice-secret")
	b := saveTestAccount(t, s, "caller:bob", 10, "bob-secret")
	c := saveTestAccount(t, s, "caller:alice", 20, "second-secret")
	if a.ID == b.ID || a.ID == c.ID || b.ID == c.ID || !validID(a.ID) {
		t.Fatal("accounts need independent opaque IDs")
	}
	if _, err := s.Get(b.Owner, a.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner Get: %v", err)
	}
	if _, ok := s.FindCookie(b.Owner, a.Cookie); ok {
		t.Fatal("cross-owner cookie lookup succeeded")
	}
	if _, ok := s.FindCookie(a.Owner, ""); ok {
		t.Fatal("empty cookie lookup succeeded")
	}
	if _, ok := s.FindCookie(a.Owner, a.Cookie+" "); ok {
		t.Fatal("nonexact cookie lookup succeeded")
	}
	if got, ok := s.FindCookie(a.Owner, a.Cookie); !ok || got != a {
		t.Fatal("owned cookie lookup failed")
	}
	a2 := saveTestAccount(t, s, a.Owner, a.UserID, "rotated-secret")
	if a2.ID != a.ID || a2.Revision != a.Revision+1 {
		t.Fatal("re-login must preserve identity and advance revision")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []Record{a2, b, c} {
		assertRecord(t, reopened, record)
	}
	if _, ok := reopened.FindCookie(a.Owner, a.Cookie); ok {
		t.Fatal("old credentials survived restart")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("account file permissions: %v", err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("account directory permissions: %v", err)
	}
}

func TestStaleLifecycleCannotOverwriteNewLogin(t *testing.T) {
	for _, operation := range []string{"update", "expire", "delete"} {
		t.Run(operation, func(t *testing.T) {
			s, path := openTestStore(t)
			old := saveTestAccount(t, s, "single", 1, "old-secret")
			current := saveTestAccount(t, s, old.Owner, old.UserID, "new-secret")
			var err error
			switch operation {
			case "update":
				_, err = s.Update(old, "stale-refresh-secret")
			case "expire":
				err = s.Expire(old)
			case "delete":
				err = s.Delete(old)
			}
			if !errors.Is(err, ErrChanged) {
				t.Fatalf("stale operation: %v", err)
			}
			assertRecord(t, s, current)
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			assertRecord(t, reopened, current)
		})
	}
}

func TestRefreshExpirationRevivalAndRevocation(t *testing.T) {
	s, path := openTestStore(t)
	original := saveTestAccount(t, s, "local", 1, "original-secret")
	refreshed, err := s.Update(original, "refreshed-secret")
	if err != nil || refreshed.ID != original.ID || refreshed.Revision != original.Revision+1 || refreshed.Cookie != "refreshed-secret" {
		t.Fatalf("refresh failed: %v", err)
	}
	if err := s.Expire(refreshed); err != nil {
		t.Fatal(err)
	}
	expired, err := s.Get(refreshed.Owner, refreshed.ID)
	if err != nil || !expired.Expired || expired.Cookie != "" || expired.Revision != refreshed.Revision+1 {
		t.Fatalf("invalid expired snapshot: %v", err)
	}
	if _, ok := s.FindCookie(refreshed.Owner, refreshed.Cookie); ok {
		t.Fatal("expired credential resolved")
	}
	if _, err := s.Update(expired, "late-secret"); !errors.Is(err, ErrChanged) {
		t.Fatalf("refresh revived expiration: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertRecord(t, reopened, expired)
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), refreshed.Cookie) {
		t.Fatalf("expired credential remains on disk: %v", err)
	}
	revived := saveTestAccount(t, reopened, expired.Owner, expired.UserID, "revived-secret")
	if revived.ID != expired.ID || revived.Expired || revived.Revision != expired.Revision+1 {
		t.Fatal("login did not revive stable account")
	}
	if err := reopened.Delete(expired); !errors.Is(err, ErrChanged) {
		t.Fatalf("stale expired logout: %v", err)
	}
	if err := reopened.Delete(revived); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(revived.Owner, revived.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked account remains available: %v", err)
	}
	if _, err := reopened.Update(revived, "late-secret"); !errors.Is(err, ErrChanged) {
		t.Fatalf("revoked account accepted update: %v", err)
	}
	if err := reopened.Delete(revived); !errors.Is(err, ErrChanged) {
		t.Fatalf("revoked account accepted stale delete: %v", err)
	}
	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Get(revived.Owner, revived.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revocation did not persist: %v", err)
	}
	newLogin := saveTestAccount(t, restarted, revived.Owner, revived.UserID, "new-login-secret")
	if newLogin.ID == revived.ID {
		t.Fatal("revoked ID was reused")
	}
	if err := restarted.Expire(revived); !errors.Is(err, ErrChanged) {
		t.Fatalf("revoked snapshot accepted expiration: %v", err)
	}
}

func TestExpiredAccountCanBeDeletedLocally(t *testing.T) {
	s, path := openTestStore(t)
	r := saveTestAccount(t, s, "local", 1, "secret")
	if err := s.Expire(r); err != nil {
		t.Fatal(err)
	}
	expired, err := s.Get(r.Owner, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(expired); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(r.Owner, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired logout did not persist: %v", err)
	}
}

func TestWriteFailureDoesNotPublish(t *testing.T) {
	for _, operation := range []string{"create", "relogin", "update", "expire", "delete"} {
		t.Run(operation, func(t *testing.T) {
			s, path := openTestStore(t)
			original := saveTestAccount(t, s, "single", 1, "existing-secret")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// A nonempty directory at the rename destination fails deterministically,
			// including when tests run with elevated filesystem privileges.
			backup := path + ".saved"
			if err := os.Rename(path, backup); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "block"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "create":
				_, err = s.Save("single", 2, "new-secret")
			case "relogin":
				_, err = s.Save("single", 1, "new-secret")
			case "update":
				_, err = s.Update(original, "new-secret")
			case "expire":
				err = s.Expire(original)
			case "delete":
				err = s.Delete(original)
			}
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("expected sanitized persistence failure")
			}
			assertRecord(t, s, original)
			if _, ok := s.FindCookie("single", "new-secret"); ok {
				t.Fatal("failed mutation published new credentials")
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(backup, path); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatalf("previous persisted state changed: %v", err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 {
				t.Fatalf("replacement temp files leaked: %v", err)
			}
			// A failed operation must leave the original revision usable.
			if _, err := s.Update(original, "recovered-secret"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMalformedPersistedStateFailsClosed(t *testing.T) {
	valid := Record{ID: "acct_" + strings.Repeat("a", 64), Owner: "single", UserID: 1, Cookie: "secret-marker", Revision: 1}
	marshal := func(records ...Record) string {
		data, err := json.Marshal(diskState{Version: 1, Records: records})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	cases := map[string]string{
		"empty":               "",
		"truncated":           "{\"cookie\":\"secret-marker",
		"null":                "null",
		"missing records":     `{"version":1}`,
		"null records":        `{"version":1,"records":null}`,
		"unsupported version": `{"version":2,"records":[]}`,
		"unknown field":       `{"version":1,"records":[],"secret-marker":true}`,
		"trailing document":   marshal(valid) + `{}`,
		"duplicate ID":        marshal(valid, valid),
	}
	for name, alter := range map[string]func(*Record){
		"empty ID":                func(r *Record) { r.ID = "" },
		"malformed ID":            func(r *Record) { r.ID = "secret-marker" },
		"nonhex ID":               func(r *Record) { r.ID = "acct_" + strings.Repeat("z", 64) },
		"empty owner":             func(r *Record) { r.Owner = "" },
		"zero user":               func(r *Record) { r.UserID = 0 },
		"negative user":           func(r *Record) { r.UserID = -1 },
		"zero revision":           func(r *Record) { r.Revision = 0 },
		"empty active cookie":     func(r *Record) { r.Cookie = "" },
		"expired with credential": func(r *Record) { r.Expired = true },
	} {
		r := valid
		alter(&r)
		cases[name] = marshal(r)
	}
	duplicateIdentity := valid
	duplicateIdentity.ID = "acct_" + strings.Repeat("b", 64)
	cases["duplicate owner user"] = marshal(valid, duplicateIdentity)
	cases["overflow revision"] = strings.Replace(marshal(valid), `"revision":1`, `"revision":18446744073709551616`, 1)
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accounts.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(path); err == nil || strings.Contains(err.Error(), "secret-marker") {
				t.Fatal("malformed state was accepted or disclosed credential content")
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != data {
				t.Fatalf("invalid state was overwritten: %v", err)
			}
		})
	}
}

func TestRevisionExhaustionDoesNotWrap(t *testing.T) {
	for _, operation := range []string{"save", "update", "expire"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accounts.json")
			r := Record{ID: "acct_" + strings.Repeat("a", 64), Owner: "single", UserID: 1, Cookie: "secret", Revision: math.MaxUint64}
			data, err := json.Marshal(diskState{Version: 1, Records: []Record{r}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "save":
				_, err = s.Save(r.Owner, r.UserID, "new-secret")
			case "update":
				_, err = s.Update(r, "new-secret")
			case "expire":
				err = s.Expire(r)
			}
			if err == nil {
				t.Fatal("revision wrapped")
			}
			assertRecord(t, s, r)
		})
	}
}

func TestOpenRejectsSymlinkAndDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secret-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, path := range []string{link, dir} {
		if _, err := Open(path); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("unsafe account store path accepted")
		}
	}
}

func TestInvalidInputsAndAlteredSnapshotsDoNotMutate(t *testing.T) {
	s, _ := openTestStore(t)
	original := saveTestAccount(t, s, "single", 1, "secret")
	for _, input := range []struct {
		owner  string
		userID int64
		cookie string
	}{
		{"", 1, "secret"}, {"single", 0, "secret"}, {"single", -1, "secret"}, {"single", 1, ""},
	} {
		if _, err := s.Save(input.owner, input.userID, input.cookie); err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if _, err := s.Update(original, ""); err == nil {
		t.Fatal("empty refresh accepted")
	}
	altered := original
	altered.Owner = "caller:other"
	if _, err := s.Update(altered, "new-secret"); !errors.Is(err, ErrChanged) {
		t.Fatalf("altered snapshot accepted: %v", err)
	}
	assertRecord(t, s, original)
}
