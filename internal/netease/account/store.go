// Package account persists explicitly selected, caller-owned upstream accounts.
// A store is owned by one process; it does not coordinate multiple writers.
package account

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound  = errors.New("account not found")
	ErrForbidden = errors.New("account forbidden")
	ErrChanged   = errors.New("account changed")
)

// Record is a value snapshot. Cookie is sensitive and must not be logged.
// Revision identifies the exact credentials observed before a network operation.
type Record struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"`
	UserID   int64  `json:"user_id"`
	Cookie   string `json:"cookie"`
	Revision uint64 `json:"revision"`
	Expired  bool   `json:"expired"`
}

// Store publishes changes only after an atomic file replacement succeeds.
// The zero value is not usable; callers must use Open.
type Store struct {
	mu      sync.RWMutex
	path    string
	records map[string]Record
}

type diskState struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// Open loads an existing store or securely initializes an absent file.
// Invalid state fails closed. Errors intentionally omit paths and file contents.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("account store path is empty")
	}
	s := &Store{path: path, records: make(map[string]Record)}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, errors.New("cannot create account store directory")
		}
		if err := s.persist(s.records); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("cannot open account store")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open account store")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("account store file changed while opening")
	}
	if err := f.Chmod(0o600); err != nil {
		return nil, errors.New("cannot secure account store")
	}
	var state diskState
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, errors.New("invalid account store data")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid account store trailing data")
	}
	if state.Version != 1 || state.Records == nil {
		return nil, errors.New("invalid account store format")
	}
	type identity struct {
		owner  string
		userID int64
	}
	seen := make(map[identity]bool, len(state.Records))
	for _, record := range state.Records {
		if !validID(record.ID) || record.Owner == "" || record.UserID <= 0 || record.Revision == 0 || (record.Expired && record.Cookie != "") || (!record.Expired && record.Cookie == "") {
			return nil, errors.New("invalid account store record")
		}
		key := identity{record.Owner, record.UserID}
		if _, exists := s.records[record.ID]; exists || seen[key] {
			return nil, errors.New("duplicate account store record")
		}
		seen[key] = true
		s.records[record.ID] = record
	}
	return s, nil
}

func validID(id string) bool {
	if len(id) != 69 || !strings.HasPrefix(id, "acct_") {
		return false
	}
	for _, c := range id[5:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Save creates or replaces this owner's account for userID. Re-login preserves
// its opaque ID, revives expired credentials, and advances the revision.
func (s *Store) Save(owner string, userID int64, cookie string) (Record, error) {
	if owner == "" || userID <= 0 || cookie == "" {
		return Record{}, errors.New("invalid account credentials")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.records {
		if current.Owner == owner && current.UserID == userID {
			if current.Revision == math.MaxUint64 {
				return Record{}, errors.New("account revision exhausted")
			}
			current.Cookie, current.Expired = cookie, false
			current.Revision++
			return s.commit(current)
		}
	}
	var id string
	for {
		var random [32]byte
		if _, err := rand.Read(random[:]); err != nil {
			return Record{}, errors.New("cannot generate account ID")
		}
		id = "acct_" + hex.EncodeToString(random[:])
		if _, exists := s.records[id]; !exists {
			break
		}
	}
	return s.commit(Record{ID: id, Owner: owner, UserID: userID, Cookie: cookie, Revision: 1})
}

// Get returns expired snapshots as well as active ones; transport decides how
// expiration affects the requested operation.
func (s *Store) Get(owner, id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	if current.Owner != owner {
		return Record{}, ErrForbidden
	}
	return current, nil
}

// FindCookie finds only active, exactly matching credentials owned by caller.
func (s *Store) FindCookie(owner, cookie string) (Record, bool) {
	if cookie == "" {
		return Record{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, current := range s.records {
		if current.Owner == owner && !current.Expired && current.Cookie == cookie {
			return current, true
		}
	}
	return Record{}, false
}

// Update replaces credentials only if old is still the current active snapshot.
func (s *Store) Update(old Record, cookie string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.compare(old); err != nil {
		return Record{}, err
	}
	if old.Expired {
		return Record{}, ErrChanged
	}
	if cookie == "" {
		return Record{}, errors.New("invalid account credentials")
	}
	if old.Revision == math.MaxUint64 {
		return Record{}, errors.New("account revision exhausted")
	}
	old.Cookie = cookie
	old.Revision++
	return s.commit(old)
}

// Expire invalidates exactly old and removes its persisted credentials.
func (s *Store) Expire(old Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.compare(old); err != nil {
		return err
	}
	if old.Revision == math.MaxUint64 {
		return errors.New("account revision exhausted")
	}
	old.Cookie, old.Expired = "", true
	old.Revision++
	_, err := s.commit(old)
	return err
}

// Delete removes exactly old. Missing or superseded snapshots are ErrChanged.
func (s *Store) Delete(old Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.compare(old); err != nil {
		return err
	}
	next := s.copyRecords()
	delete(next, old.ID)
	if err := s.persist(next); err != nil {
		return err
	}
	s.records = next
	return nil
}

// compare requires the lock and compares the whole value to reject altered
// snapshots as well as stale revisions. No credential enters error messages.
func (s *Store) compare(old Record) error {
	current, exists := s.records[old.ID]
	if !exists || current != old {
		return ErrChanged
	}
	return nil
}

func (s *Store) copyRecords() map[string]Record {
	next := make(map[string]Record, len(s.records)+1)
	for id, record := range s.records {
		next[id] = record
	}
	return next
}

// commit requires the write lock.
func (s *Store) commit(record Record) (Record, error) {
	next := s.copyRecords()
	next[record.ID] = record
	if err := s.persist(next); err != nil {
		return Record{}, err
	}
	s.records = next
	return record, nil
}

func (s *Store) persist(records map[string]Record) error {
	state := diskState{Version: 1, Records: make([]Record, 0, len(records))}
	for _, record := range records {
		state.Records = append(state.Records, record)
	}
	sort.Slice(state.Records, func(i, j int) bool { return state.Records[i].ID < state.Records[j].ID })
	f, err := os.CreateTemp(filepath.Dir(s.path), ".kite-accounts-*")
	if err != nil {
		return errors.New("cannot create account store replacement")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return errors.New("cannot secure account store replacement")
	}
	if err := json.NewEncoder(f).Encode(state); err != nil {
		return errors.New("cannot write account store replacement")
	}
	if err := f.Sync(); err != nil {
		return errors.New("cannot sync account store replacement")
	}
	if err := f.Close(); err != nil {
		return errors.New("cannot close account store replacement")
	}
	// Rename is the commit point. No fallible operation may follow it: callers
	// must never receive a failed mutation after the new state is on disk.
	if err := os.Rename(f.Name(), s.path); err != nil {
		return errors.New("cannot replace account store")
	}
	return nil
}
