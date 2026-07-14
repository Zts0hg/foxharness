package permission

import "sync"

// Mode is the user-selected approval behavior for interactive tool calls.
type Mode string

const (
	ModeAsk        Mode = "ask"
	ModeApprove    Mode = "approve"
	ModeFullAccess Mode = "full_access"
)

// NormalizeMode maps persisted or user-provided values to a supported mode.
func NormalizeMode(mode string) Mode {
	switch Mode(mode) {
	case ModeApprove:
		return ModeApprove
	case ModeFullAccess:
		return ModeFullAccess
	default:
		return ModeAsk
	}
}

// State stores the selected mode, effective mode, and process-local grants.
type State struct {
	mu sync.Mutex

	selected       Mode
	effective      Mode
	fullRemembered bool
	grants         map[GrantKey]Grant
}

// NewState creates permission state from persisted settings.
func NewState(selected Mode, fullAccessRemembered bool) *State {
	selected = NormalizeMode(string(selected))
	effective := selected
	if selected == ModeFullAccess && !fullAccessRemembered {
		effective = ModeAsk
	}
	return &State{
		selected:       selected,
		effective:      effective,
		fullRemembered: fullAccessRemembered,
		grants:         make(map[GrantKey]Grant),
	}
}

// Snapshot returns an immutable view of current permission state.
func (s *State) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		SelectedMode:           s.selected,
		EffectiveMode:          s.effective,
		FullAccessRemembered:   s.fullRemembered,
		SessionGrantCount:      len(s.grants),
		FullAccessNeedsWarning: s.selected == ModeFullAccess && s.effective != ModeFullAccess,
	}
}

// SetSelected changes both selected and effective mode except for unremembered
// Full Access, which stays effectively Ask until explicitly activated.
func (s *State) SetSelected(mode Mode, fullAccessRemembered bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = NormalizeMode(string(mode))
	s.fullRemembered = fullAccessRemembered
	s.effective = s.selected
	if s.selected == ModeFullAccess && !s.fullRemembered {
		s.effective = ModeAsk
	}
}

// ActivateFullAccess enables Full Access for the current TUI process.
func (s *State) ActivateFullAccess(remember bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = ModeFullAccess
	s.effective = ModeFullAccess
	if remember {
		s.fullRemembered = true
	}
}

// AddGrant stores a typed session grant.
func (s *State) AddGrant(grant Grant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grants == nil {
		s.grants = make(map[GrantKey]Grant)
	}
	s.grants[grant.Key] = grant
}

// MatchingGrant reports whether request is covered by a session grant.
func (s *State) MatchingGrant(request Request) (Grant, bool) {
	key := GrantKeyFor(request)
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[key]
	return grant, ok
}

// ClearGrants removes every process-local session grant.
func (s *State) ClearGrants() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.grants)
	s.grants = make(map[GrantKey]Grant)
	return n
}

// Snapshot describes visible permission state.
type Snapshot struct {
	SelectedMode           Mode
	EffectiveMode          Mode
	FullAccessRemembered   bool
	FullAccessNeedsWarning bool
	SessionGrantCount      int
}
