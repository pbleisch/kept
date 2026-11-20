package ledger

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// SessionsRef is the ref that carries session records: every thread's receipt
// and every payload a receipt names. [Ref] carries everything else, and is
// what a clone fetches and a person publishes; this one is neither.
//
// The name is the guarantee, and three prefixes were refused for it. Not
// refs/kept/ledger…, because [FetchRefspec]'s * would match it and every clone
// would fetch the sessions record from origin. Not refs/kept/threads/…,
// because [AnchoredThreads] enumerates that prefix as the land queue. Not
// refs/kept/upstream/…, which holds copies of what a remote holds and never a
// record kept writes. See docs/decision/0089.
const SessionsRef = "refs/kept/sessions"

// SessionsUpstreamRef is where a fetch or a push of the named sessions remote
// leaves its copy of what that remote holds. Nothing reads a session record
// from here and nothing advances [SessionsRef] onto it; it exists so that
// doctor can say what the remote did not have, with no network call, for the
// reason [UpstreamRef] lets it say that about the ledger.
const SessionsUpstreamRef = "refs/kept/upstream/sessions"

// SessionsFetchRefspec maps the named remote's sessions ref onto
// [SessionsUpstreamRef]. It is configured on that remote and no other, and it
// is what makes git update the upstream copy when the sessions ref is pushed —
// measured for the ledger in docs/decision/0075, and the same mechanism here.
// It is a single ref rather than a pattern, so it cannot match anything else.
const SessionsFetchRefspec = "+" + SessionsRef + ":" + SessionsUpstreamRef

// SessionsRemoteKey is the git config key naming the one remote session
// records may be pushed to. It is repository configuration, so it is never on
// main, never cloned and never published.
const SessionsRemoteKey = "kept.sessionsRemote"

// SessionsPushCommand is the one line that copies session records to the named
// remote, printed by doctor so a person can copy it rather than compose it.
// Unforced, for the reason [PushCommand] is: the sessions ref only moves
// forward, so an honest push is a fast-forward, and a refusal is the remote
// saying something else reached it first. kept never runs it.
func SessionsPushCommand(remote string) string {
	return "git push " + remote + " " + SessionsRef + ":" + SessionsRef
}

// ErrNotARemote is a sessions remote that names no remote this repository has,
// so no push could reach it.
var ErrNotARemote = errors.New("not a remote here")

// ErrPublishedRemote is a sessions remote that is where the ledger is published:
// origin by name, or a remote whose URL is origin's. kept cannot tell a private
// remote from a public one, and this is the one remote it can tell is the
// wrong one, because [PushCommand] names it.
var ErrPublishedRemote = errors.New("the remote the ledger is published to")

// SessionsConfig is what [ConfigureSessions] did.
type SessionsConfig struct {
	// Remote is the remote session records go to, or empty where none is
	// named.
	Remote string
	// Named is whether this call wrote [SessionsRemoteKey].
	Named bool
	// Fetch is whether this call added [SessionsFetchRefspec] to the remote.
	Fetch FetchConfig
}

// ConfigureSessions names the remote session records may be pushed to, and
// makes a fetch or a push of it keep [SessionsUpstreamRef]. An empty remote
// names nothing new: it makes sure the remote already named has the refspec,
// which is what kept init does on every run.
//
// It refuses a remote that is not one ([ErrNotARemote]) and the remote the
// ledger is published to ([ErrPublishedRemote]), before writing anything. It
// never removes a refspec and never touches origin's. See docs/decision/0089.
func ConfigureSessions(repository, remote string) (SessionsConfig, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return SessionsConfig{}, ErrNotRepository
		}
		return SessionsConfig{}, err
	}
	named, err := s.configValue(SessionsRemoteKey)
	if err != nil {
		return SessionsConfig{}, err
	}
	if remote == "" {
		remote = named
	}
	if remote == "" {
		return SessionsConfig{}, nil
	}
	if err := s.privateRemote(remote); err != nil {
		return SessionsConfig{Remote: named}, err
	}

	done := SessionsConfig{Remote: remote, Fetch: FetchUnchanged}
	if named != remote {
		if _, err := s.git(nil, nil, "config", SessionsRemoteKey, remote); err != nil {
			return SessionsConfig{Remote: named}, err
		}
		done.Named = true
	}
	key := "remote." + remote + ".fetch"
	held, err := s.configValues(key)
	if err != nil {
		return done, err
	}
	if !slices.Contains(held, SessionsFetchRefspec) {
		if _, err := s.git(nil, nil, "config", "--add", key, SessionsFetchRefspec); err != nil {
			return done, err
		}
		done.Fetch = FetchAdded
	}
	return done, nil
}

// privateRemote is the refusal [ConfigureSessions] makes before it writes.
func (s *RefStore) privateRemote(remote string) error {
	if remote == "origin" {
		return fmt.Errorf("%s is %w", remote, ErrPublishedRemote)
	}
	url, err := s.configValue("remote." + remote + ".url")
	if err != nil {
		return err
	}
	if url == "" {
		return fmt.Errorf("%s is %w", remote, ErrNotARemote)
	}
	origin, err := s.origin()
	if err != nil {
		return err
	}
	if origin != "" && origin == url {
		return fmt.Errorf("%s has origin's URL, so it is %w", remote, ErrPublishedRemote)
	}
	return nil
}

// SessionsState is what this repository's own refs and config say about where
// its session records live. Every field is local; nothing here contacts a
// remote.
type SessionsState struct {
	// Held is the value [SessionsRef] holds, and is empty when nothing has
	// recorded a session here.
	Held string
	// Remote is the remote [SessionsRemoteKey] names, or empty.
	Remote string
	// URL is that remote's URL, and is empty when it is not a remote here.
	URL string
	// Published says the named remote is origin, or has origin's URL.
	Published bool
	// Tracked says the named remote carries [SessionsFetchRefspec], so a push
	// to it updates Upstream. Without it the outbound comparison never has a
	// value, whatever has been pushed.
	Tracked bool
	// Upstream is the value [SessionsUpstreamRef] holds — what the named
	// remote held as of the last fetch or push — or empty when nothing here
	// has heard from it.
	Upstream string
	// Ahead and Behind count the commits each of Held and Upstream holds that
	// the other cannot reach.
	Ahead, Behind int
}

// Unbacked is session records held here with no remote named, so they exist
// only on this machine.
func (s SessionsState) Unbacked() bool { return s.Held != "" && s.Remote == "" }

// Unpublished is session records the named remote did not have as of the last
// fetch or push, and nothing the other way: the state one fast-forward
// [SessionsPushCommand] resolves. An upstream value nothing has written is not
// this; it is [SessionsState.Unheard].
func (s SessionsState) Unpublished() bool {
	return s.Upstream != "" && s.Ahead > 0 && s.Behind == 0
}

// Unheard is session records held here, a usable remote named, and no value for
// what that remote holds: *nobody has asked*, rather than *the remote does not
// have them*.
func (s SessionsState) Unheard() bool {
	return s.Held != "" && s.URL != "" && !s.Published && s.Tracked && s.Upstream == ""
}

// Diverged is a sessions ref and a remote copy that have both moved, so the
// push would be refused.
func (s SessionsState) Diverged() bool { return s.Ahead > 0 && s.Behind > 0 }

// Sessions reports where this repository's session records live. It is local
// config reads and one rev-list, and never contacts a remote.
func Sessions(repository string) (SessionsState, error) {
	s := NewRefStore(repository)
	if _, _, err := s.revision(); err != nil {
		if isNotRepository(err) {
			return SessionsState{}, ErrNotRepository
		}
		return SessionsState{}, err
	}
	var state SessionsState
	var err error
	if state.Held, _, err = s.revisionOf(SessionsRef); err != nil {
		return state, err
	}
	if state.Remote, err = s.configValue(SessionsRemoteKey); err != nil || state.Remote == "" {
		return state, err
	}
	if state.URL, err = s.configValue("remote." + state.Remote + ".url"); err != nil {
		return state, err
	}
	origin, err := s.origin()
	if err != nil {
		return state, err
	}
	state.Published = state.Remote == "origin" || (state.URL != "" && state.URL == origin)
	fetches, err := s.configValues("remote." + state.Remote + ".fetch")
	if err != nil {
		return state, err
	}
	state.Tracked = slices.Contains(fetches, SessionsFetchRefspec)

	if state.Upstream, _, err = s.value(SessionsUpstreamRef); err != nil || state.Upstream == "" || state.Held == "" {
		return state, err
	}
	state.Ahead, state.Behind, err = s.apart(state.Held, state.Upstream)
	return state, err
}

// configValue reads one git config value. An unset key is exit 1 and is the
// answer, not an error.
func (s *RefStore) configValue(key string) (string, error) {
	out, err := s.git(nil, nil, "config", "--get", key)
	if err != nil {
		if isExit(err, 1) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// configValues reads every value of a multi-valued git config key.
func (s *RefStore) configValues(key string) ([]string, error) {
	out, err := s.git(nil, nil, "config", "--get-all", key)
	if err != nil {
		if isExit(err, 1) {
			return nil, nil
		}
		return nil, err
	}
	return strings.Fields(string(out)), nil
}
