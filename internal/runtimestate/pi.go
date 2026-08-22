package runtimestate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PiCredentialEnvironment is the one child environment name a projected Pi
// configuration refers to. kept owns the name rather than taking the
// operator's, because two places choosing it is one place it can disagree:
// the projected entry and the injected value have to be the same name or the
// runtime silently starts unauthenticated. It is deliberately not a
// provider-native name such as ANTHROPIC_API_KEY, which pi-ai also resolves on
// its own — a value nothing but this projection reads cannot authenticate a
// provider the projection did not select.
const PiCredentialEnvironment = "KEPT_PI_CREDENTIAL"

// piCredentialReference is what a projected credential entry stores in place of
// a credential. It is Pi's documented environment-reference syntax for a
// configuration value, so the projected file is a placeholder and the secret
// stays in the child environment that decision 0052's credential channel names.
const piCredentialReference = "$" + PiCredentialEnvironment

// PiDefaultEndpoint is the authority an unattended repair uses when the caller
// authorizes none. Loopback is the default because a self-hosted endpoint on
// the same machine is the one an unattended run can reach without depending on
// a network that may not be there.
const PiDefaultEndpoint = "localhost:1234"

// The three files a projected Pi state directory holds. Pi reads several more
// under its agent directory; the projection creates none of them, and Pi
// creates what it needs inside the same private directory as it runs.
const (
	piAuthFile     = "auth.json"
	piModelsFile   = "models.json"
	piSettingsFile = "settings.json"
)

// piMaxSourceBytes bounds each source file the projection reads. A
// configuration file is small, and an unbounded read of an operator-approved
// path is still an unbounded read.
const piMaxSourceBytes = 1 << 20

// PiRequest is one Pi configuration projection. Every path is absolute, and
// the projection reads only Source: it never consults the parent environment,
// a keychain, the corpus or the ledger to decide what to copy.
type PiRequest struct {
	// Source is the operator-approved Pi agent directory the projection reads.
	// It is the only place a credential entry or provider definition may come
	// from, and it is read and never written.
	Source string

	// Dir is the private per-run destination. It must not exist: the
	// projection creates it, so a directory already there is somebody else's
	// state rather than this run's.
	Dir string

	// Box is the repair worktree. Neither Source nor Dir may be inside it. A
	// source there is content a repair agent authored, which would let a
	// repair choose the endpoint and credential its own next run receives; a
	// destination there is a credential reference in the tree whose diff is
	// collected and whose contents may land.
	Box string

	// Provider and Model are the explicit selection. The projection carries
	// exactly this provider's credential entry and definition, and exactly
	// this model, and nothing else the source happens to hold.
	Provider string
	Model    string

	// Endpoints are the authorities authorized for unattended repair, as
	// host:port. Empty selects PiDefaultEndpoint alone. Every entry — the
	// default included — must be loopback or a private address: authorizing a
	// public host is what a self-hosted endpoint is not.
	Endpoints []string
}

// PiState is a materialized projection. It is obtainable only from ProjectPi,
// which is the point: an adapter that took a directory name could be pointed at
// the operator's own Pi state, and the empty-directory rule this replaced
// existed to make that impossible.
type PiState struct {
	dir string
}

// Dir is the private directory the child receives as both its home and its Pi
// agent directory.
func (s *PiState) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Remove discards the projection. It is idempotent, and a caller defers it
// beside the box's own discard: the copy has no reason to outlive the tree the
// repair ran in.
func (s *PiState) Remove() error {
	if s == nil || s.dir == "" {
		return nil
	}
	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("discard pi projection at %s: %w", s.dir, err)
	}
	return nil
}

// ProjectPi materializes the minimal writable Pi configuration one repair run
// needs, and refuses rather than narrowing when the source holds something the
// projection cannot certify.
//
// Writable is not a convenience. Pi 0.84.0's auth store, models store,
// settings store and trust store all take a proper-lockfile lock to *read*,
// and the lock is a directory created beside the file it guards, so a
// read-only projection fails on EACCES from mkdir rather than working with
// fewer writes. It fails in three different ways depending on which store got
// there first — the settings path warns and carries on, the auth path exits
// uncaught, and AuthStorage.reload catches the same failure and keeps its empty
// snapshot, which is an unauthenticated run that never says so. Copy-on-write
// into a private directory is the shape that works, and the copy is discarded
// with the box. See the real-Pi test in internal/thread.
func ProjectPi(req PiRequest) (*PiState, error) {
	plan, err := planPi(req)
	if err != nil {
		return nil, err
	}
	return writePi(plan)
}

// piPlan is a projection decided but not yet written. Everything that can be
// refused is refused before the destination exists, so a refusal never leaves a
// half-made state directory behind for the next run to inherit.
type piPlan struct {
	dir      string
	auth     []byte
	models   []byte
	settings []byte
}

func planPi(req PiRequest) (piPlan, error) {
	for _, field := range []struct{ name, value string }{
		{"provider", req.Provider},
		{"model", req.Model},
	} {
		if field.value == "" {
			return piPlan{}, fmt.Errorf("pi projection %s is unset", field.name)
		}
		if strings.HasPrefix(field.value, "-") || strings.ContainsAny(field.value, "\x00\r\n") {
			return piPlan{}, fmt.Errorf("pi projection %s is not a direct value: %q", field.name, field.value)
		}
	}
	authorities, err := piAuthorities(req.Endpoints)
	if err != nil {
		return piPlan{}, err
	}
	source, dir, err := piLocations(req)
	if err != nil {
		return piPlan{}, err
	}
	credential, err := piCredentialEntry(source, req.Provider)
	if err != nil {
		return piPlan{}, err
	}
	provider, err := piProviderEntry(source, req.Provider, req.Model, authorities)
	if err != nil {
		return piPlan{}, err
	}

	auth, err := json.MarshalIndent(map[string]piStoredCredential{req.Provider: credential}, "", "  ")
	if err != nil {
		return piPlan{}, fmt.Errorf("encode pi projected credential: %w", err)
	}
	models, err := json.MarshalIndent(piModelsFileContent{
		Providers: map[string]piProviderConfig{req.Provider: provider},
	}, "", "  ")
	if err != nil {
		return piPlan{}, fmt.Errorf("encode pi projected provider: %w", err)
	}
	settings, err := json.MarshalIndent(piProjectedSettings(req.Provider, req.Model), "", "  ")
	if err != nil {
		return piPlan{}, fmt.Errorf("encode pi projected settings: %w", err)
	}
	return piPlan{dir: dir, auth: auth, models: models, settings: settings}, nil
}

func writePi(plan piPlan) (*PiState, error) {
	// Mkdir rather than MkdirAll: the destination is this run's and must not
	// already exist, and a mode is applied explicitly because the process
	// umask would otherwise decide who can read a credential reference.
	if err := os.Mkdir(plan.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create pi projection at %s: %w", plan.dir, err)
	}
	state := &PiState{dir: plan.dir}
	if err := os.Chmod(plan.dir, 0o700); err != nil {
		return nil, piDiscardAfterCreate(state, fmt.Errorf("set pi projection mode at %s: %w", plan.dir, err))
	}
	for _, file := range []struct {
		name string
		data []byte
	}{
		{piAuthFile, plan.auth},
		{piModelsFile, plan.models},
		{piSettingsFile, plan.settings},
	} {
		path := filepath.Join(plan.dir, file.name)
		if err := os.WriteFile(path, append(file.data, '\n'), 0o600); err != nil {
			return nil, piDiscardAfterCreate(state, fmt.Errorf("write pi projection %s: %w", file.name, err))
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, piDiscardAfterCreate(state, fmt.Errorf("set pi projection mode %s: %w", file.name, err))
		}
	}
	return state, nil
}

func piDiscardAfterCreate(state *PiState, cause error) error {
	if err := state.Remove(); err != nil {
		return fmt.Errorf("%w; %v", cause, err)
	}
	return cause
}

// piLocations resolves the three paths and refuses every arrangement in which
// the projection would read what a repair wrote or write where a repair reads.
func piLocations(req PiRequest) (source, dir string, err error) {
	for _, path := range []struct{ name, value string }{
		{"source", req.Source},
		{"destination", req.Dir},
		{"box", req.Box},
	} {
		if path.value == "" {
			return "", "", fmt.Errorf("pi projection %s is unset", path.name)
		}
		if !filepath.IsAbs(path.value) {
			return "", "", fmt.Errorf("pi projection %s is not an explicit absolute path: %s", path.name, path.value)
		}
	}
	source, err = piResolvedDirectory("source", req.Source)
	if err != nil {
		return "", "", err
	}
	box, err := piResolvedDirectory("box", req.Box)
	if err != nil {
		return "", "", err
	}
	// The destination does not exist yet, so only its parent can be resolved.
	// Resolving the parent is what stops a symlinked parent from placing the
	// projection somewhere the checks below already refused.
	clean := filepath.Clean(req.Dir)
	parent, err := piResolvedDirectory("destination parent", filepath.Dir(clean))
	if err != nil {
		return "", "", err
	}
	dir = filepath.Join(parent, filepath.Base(clean))

	// The arrangement is decided before what is on disk at the destination,
	// because an overlap is wrong whether or not anything is there yet.
	if piWithin(box, source) {
		return "", "", fmt.Errorf("pi projection source is inside the box: %s", source)
	}
	if piWithin(box, dir) {
		return "", "", fmt.Errorf("pi projection destination is inside the box: %s", dir)
	}
	if piWithin(source, dir) || piWithin(dir, source) {
		return "", "", fmt.Errorf("pi projection source and destination overlap: %s and %s", source, dir)
	}
	if _, err := os.Lstat(dir); err == nil {
		return "", "", fmt.Errorf("pi projection destination already exists: %s", dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect pi projection destination %s: %w", dir, err)
	}
	return source, dir, nil
}

func piResolvedDirectory(name, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve pi projection %s %s: %w", name, path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect pi projection %s %s: %w", name, path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("pi projection %s is not a directory: %s", name, path)
	}
	return resolved, nil
}

func piWithin(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// piRead reads one source file, refusing anything that is not a plain regular
// file. A symlink is refused rather than followed: the source directory is the
// bound on what may be projected, and a link inside it is a way out of that
// bound to a path the operator never approved.
func piRead(source, name string) ([]byte, error) {
	path := filepath.Join(source, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read pi projection source %s: %w", name, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return nil, fmt.Errorf("pi projection source %s is a symlink", name)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("pi projection source %s is not a regular file", name)
	case info.Size() > piMaxSourceBytes:
		return nil, fmt.Errorf("pi projection source %s exceeds %d bytes", name, piMaxSourceBytes)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pi projection source %s: %w", name, err)
	}
	return src, nil
}

// piStoredCredential is one auth.json entry. The projection writes only the
// api_key form, and writes the reference rather than the key.
type piStoredCredential struct {
	Type string            `json:"type"`
	Key  string            `json:"key,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

func piCredentialEntry(source, provider string) (piStoredCredential, error) {
	src, err := piRead(source, piAuthFile)
	if err != nil {
		return piStoredCredential{}, err
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(src, &stored); err != nil {
		return piStoredCredential{}, fmt.Errorf("decode pi projection source %s: %w", piAuthFile, err)
	}
	raw, found := stored[provider]
	if !found {
		return piStoredCredential{}, fmt.Errorf("pi projection source holds no credential for provider %s", provider)
	}
	var tagged struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &tagged); err != nil {
		return piStoredCredential{}, fmt.Errorf("decode pi projection source credential for provider %s: %w", provider, err)
	}
	switch tagged.Type {
	case "api_key":
	case "oauth":
		// An OAuth entry auto-refreshes and persists what it is given back, so
		// a copy that refreshes in a box can strand the operator's own login.
		// It is the hazard that keeps the Codex adapter closed, and it does not
		// become safe by arriving through a different runtime's file.
		return piStoredCredential{}, fmt.Errorf("pi projection refuses the oauth credential for provider %s: a refreshed copy can strand the operator's login", provider)
	default:
		return piStoredCredential{}, fmt.Errorf("pi projection does not know credential type %q for provider %s", tagged.Type, provider)
	}
	entry, err := piDecode[piStoredCredential](raw)
	if err != nil {
		return piStoredCredential{}, fmt.Errorf("decode pi projection source credential for provider %s: %w", provider, err)
	}
	for name, value := range entry.Env {
		if !piEnvironmentName(name) {
			return piStoredCredential{}, fmt.Errorf("pi projection source credential names an invalid environment value: %q", name)
		}
		if name == PiCredentialEnvironment {
			// Pi resolves a reference against the entry's own env before the
			// child environment, so this name in the file would make the file
			// the credential channel rather than the reference to one.
			return piStoredCredential{}, fmt.Errorf("pi projection source credential shadows %s", PiCredentialEnvironment)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return piStoredCredential{}, fmt.Errorf("pi projection source credential environment %s is not a direct value", name)
		}
	}
	return piStoredCredential{Type: "api_key", Key: piCredentialReference, Env: entry.Env}, nil
}

// piDecode refuses a field the projection does not model. Re-encoding an
// object from a partial understanding of it is how a projection silently drops
// the one key that decided where a request went or how it was authenticated;
// the version pin on the runtime is what makes a field added upstream arrive
// as a refusal here rather than as a quiet omission.
func piDecode[T any](raw json.RawMessage) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	return value, nil
}

// piModelsFileContent is models.json. Only the selected provider appears.
type piModelsFileContent struct {
	Providers map[string]piProviderConfig `json:"providers"`
}

// piProviderConfig is one models.json provider, and it is every field Pi
// 0.84.0 declares — a source that carries another is refused rather than
// projected without it. compat and each model definition stay raw, because they
// carry the request shape a self-hosted endpoint needs and nothing here has to
// read them.
type piProviderConfig struct {
	Name           string                     `json:"name,omitempty"`
	BaseURL        string                     `json:"baseUrl,omitempty"`
	APIKey         string                     `json:"apiKey,omitempty"`
	API            string                     `json:"api,omitempty"`
	OAuth          string                     `json:"oauth,omitempty"`
	Headers        map[string]string          `json:"headers,omitempty"`
	Compat         json.RawMessage            `json:"compat,omitempty"`
	AuthHeader     *bool                      `json:"authHeader,omitempty"`
	Models         []json.RawMessage          `json:"models,omitempty"`
	ModelOverrides map[string]json.RawMessage `json:"modelOverrides,omitempty"`
}

// piModelConfig is the part of a model definition or override the projection
// checks. Everything else in the definition is carried through unread.
type piModelConfig struct {
	ID      string            `json:"id"`
	BaseURL string            `json:"baseUrl,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func piProviderEntry(source, provider, model string, authorities map[string]bool) (piProviderConfig, error) {
	src, err := piRead(source, piModelsFile)
	if err != nil {
		return piProviderConfig{}, err
	}
	var file struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(src, &file); err != nil {
		return piProviderConfig{}, fmt.Errorf("decode pi projection source %s: %w", piModelsFile, err)
	}
	raw, found := file.Providers[provider]
	if !found {
		return piProviderConfig{}, fmt.Errorf("pi projection source defines no provider %s", provider)
	}
	config, err := piDecode[piProviderConfig](raw)
	if err != nil {
		return piProviderConfig{}, fmt.Errorf("decode pi projection source provider %s: %w", provider, err)
	}
	if config.OAuth != "" {
		return piProviderConfig{}, fmt.Errorf("pi projection refuses provider %s: it authenticates with oauth", provider)
	}
	if config.APIKey != "" {
		// A provider-level key is a second credential channel, and 0052 admits
		// one. Dropping it silently would start an unauthenticated session.
		return piProviderConfig{}, fmt.Errorf("pi projection refuses provider %s: it declares an inline apiKey rather than using %s", provider, PiCredentialEnvironment)
	}
	if config.BaseURL == "" {
		// Without one, the endpoint comes from Pi's own provider catalogue and
		// the projection cannot certify what it did not read.
		return piProviderConfig{}, fmt.Errorf("pi projection refuses provider %s: it names no baseUrl to authorize", provider)
	}
	if err := piEndpoint("provider "+provider+" baseUrl", config.BaseURL, authorities); err != nil {
		return piProviderConfig{}, err
	}
	if err := piHeaders("provider "+provider, config.Headers); err != nil {
		return piProviderConfig{}, err
	}
	selected, overrides, err := piSelectModel(config, provider, model, authorities)
	if err != nil {
		return piProviderConfig{}, err
	}
	config.Models = []json.RawMessage{selected}
	config.ModelOverrides = overrides
	return config, nil
}

func piSelectModel(config piProviderConfig, provider, model string, authorities map[string]bool) (json.RawMessage, map[string]json.RawMessage, error) {
	if len(config.Models) == 0 {
		// A run with --offline resolves a model from what is projected and from
		// nothing else, so a provider whose models arrive from a catalogue
		// refresh has no model at all inside the box.
		return nil, nil, fmt.Errorf("pi projection refuses provider %s: it declares no models", provider)
	}
	var selected json.RawMessage
	for _, raw := range config.Models {
		var definition piModelConfig
		if err := json.Unmarshal(raw, &definition); err != nil {
			return nil, nil, fmt.Errorf("decode pi projection source model for provider %s: %w", provider, err)
		}
		if definition.ID != model {
			continue
		}
		if err := piModelEndpoints("model "+model, definition, authorities); err != nil {
			return nil, nil, err
		}
		selected = raw
		break
	}
	if selected == nil {
		return nil, nil, fmt.Errorf("pi projection source provider %s defines no model %s", provider, model)
	}
	// Overrides are looked up by exact model id, so carrying the selected
	// model's entry and dropping the rest changes nothing that applies.
	raw, found := config.ModelOverrides[model]
	if !found {
		return selected, nil, nil
	}
	var override piModelConfig
	if err := json.Unmarshal(raw, &override); err != nil {
		return nil, nil, fmt.Errorf("decode pi projection source model override for %s: %w", model, err)
	}
	if err := piModelEndpoints("model override "+model, override, authorities); err != nil {
		return nil, nil, err
	}
	return selected, map[string]json.RawMessage{model: raw}, nil
}

func piModelEndpoints(field string, definition piModelConfig, authorities map[string]bool) error {
	if definition.BaseURL != "" {
		if err := piEndpoint(field+" baseUrl", definition.BaseURL, authorities); err != nil {
			return err
		}
	}
	return piHeaders(field, definition.Headers)
}

func piHeaders(field string, headers map[string]string) error {
	for name, value := range headers {
		if err := piLiteral(fmt.Sprintf("%s header %q", field, name), value); err != nil {
			return err
		}
	}
	return nil
}

// piProjectedSettings is authored rather than copied. The operator's own
// settings file is a surface of package, extension, skill, prompt, theme,
// shell and proxy knobs, and copying any of it means auditing all of it on
// every Pi release; a projection states what it needs instead.
func piProjectedSettings(provider, model string) map[string]any {
	return map[string]any{
		"defaultProvider": provider,
		"defaultModel":    model,
		// Nothing in the repair tree may become configuration. The argv already
		// disables discovery; this closes the startup path that would otherwise
		// ask a session whether to trust a box that carries a .pi directory.
		"defaultProjectTrust": "never",
		// Both default to reporting home on some paths, and a confined run's
		// only answer to an unexpected host is a canary failure.
		"enableInstallTelemetry": false,
		"enableAnalytics":        false,
	}
}

// piAuthorities normalizes the authorized endpoint set. An empty configuration
// is the loopback default rather than an error, because the default is the
// safest value rather than a guess at the operator's intent.
func piAuthorities(configured []string) (map[string]bool, error) {
	names := configured
	if len(names) == 0 {
		names = []string{PiDefaultEndpoint}
	}
	authorities := make(map[string]bool, len(names))
	for _, name := range names {
		authority, err := piAuthority(name)
		if err != nil {
			return nil, err
		}
		authorities[authority] = true
	}
	return authorities, nil
}

// piAuthority normalizes one host:port and refuses anything that is not
// self-hosted. The rule is structural rather than a list of hosts: a public
// name is refused whatever the configuration says, so an endpoint that is not
// the operator's own machine or network cannot be selected by mistake or by a
// configuration edit.
func piAuthority(value string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("pi endpoint %q is not host:port", value)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("pi endpoint %q has no valid port", value)
	}
	host = strings.ToLower(host)
	if host == "localhost" {
		return net.JoinHostPort(host, port), nil
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return "", fmt.Errorf("pi endpoint host %q is a name rather than a self-hosted address", host)
	}
	if !address.IsLoopback() && !address.IsPrivate() && !address.IsLinkLocalUnicast() {
		return "", fmt.Errorf("pi endpoint host %q is neither loopback nor a private address", host)
	}
	return net.JoinHostPort(address.String(), port), nil
}

func piEndpoint(field, value string, authorities map[string]bool) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("pi %s is not a URL: %q", field, value)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("pi %s scheme %q is neither http nor https", field, parsed.Scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("pi %s carries credentials in its URL", field)
	}
	if parsed.Port() == "" {
		// A missing port would have to be matched against a guessed default,
		// and an authorization that guesses is not one.
		return fmt.Errorf("pi %s names no explicit port: %q", field, value)
	}
	authority, err := piAuthority(parsed.Host)
	if err != nil {
		return fmt.Errorf("pi %s: %w", field, err)
	}
	if !authorities[authority] {
		return fmt.Errorf("pi %s %s is not an authorized repair endpoint", field, authority)
	}
	return nil
}

// piLiteral refuses a Pi configuration value that Pi would resolve to something
// other than itself. A leading "!" is executed through a shell, which is
// decision 0038's refusal arriving through a JSON file rather than a run line;
// a "$NAME" reference reads the child environment, where the only name kept
// sets is the credential's.
func piLiteral(field, value string) error {
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("pi %s is not a direct value", field)
	}
	if strings.HasPrefix(value, "!") {
		return fmt.Errorf("pi %s is a shell command", field)
	}
	if piEnvironmentReference(value) {
		return fmt.Errorf("pi %s reads a child environment value", field)
	}
	return nil
}

// piEnvironmentReference mirrors Pi's own template parse: "$$" and "$!" escape,
// "${NAME}" and "$NAME" reference. It is replicated rather than approximated
// because a value kept reads as literal and Pi reads as a reference is a value
// that resolves to something nobody reviewed.
func piEnvironmentReference(value string) bool {
	for i := 0; i < len(value); {
		if value[i] != '$' {
			i++
			continue
		}
		if i+1 >= len(value) {
			return false
		}
		switch value[i+1] {
		case '$', '!':
			i += 2
		case '{':
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				i++
				continue
			}
			if piEnvironmentName(value[i+2 : i+2+end]) {
				return true
			}
			i += 3 + end
		default:
			if piEnvironmentNamePrefix(value[i+1:]) > 0 {
				return true
			}
			i++
		}
	}
	return false
}

func piEnvironmentName(name string) bool {
	return name != "" && piEnvironmentNamePrefix(name) == len(name)
}

func piEnvironmentNamePrefix(value string) int {
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return i
		}
	}
	return len(value)
}
