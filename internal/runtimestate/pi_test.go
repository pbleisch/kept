package runtimestate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture names a workshop endpoint on loopback and a second provider that
// is everything the projection must refuse: an OAuth credential, a public host,
// and a model nobody selected. Every value here is invented for the test.
const (
	fixtureProvider = "workshop"
	fixtureModel    = "workshop-medium"
	fixtureOther    = "elsewhere"
	fixtureKey      = "fixture-source-side-value"
)

func fixtureAuth() map[string]any {
	return map[string]any{
		fixtureProvider: map[string]any{"type": "api_key", "key": fixtureKey},
		fixtureOther:    map[string]any{"type": "oauth", "refresh": "r", "access": "a", "expires": 1},
	}
}

func fixtureModels() map[string]any {
	return map[string]any{
		"providers": map[string]any{
			fixtureProvider: map[string]any{
				"name":    "Workshop",
				"baseUrl": "http://localhost:1234/v1",
				"api":     "openai-completions",
				"models": []any{
					map[string]any{"id": fixtureModel, "name": "Workshop Medium"},
					map[string]any{"id": "workshop-large", "name": "Workshop Large"},
				},
				"modelOverrides": map[string]any{
					fixtureModel:     map[string]any{"maxTokens": 4096},
					"workshop-large": map[string]any{"maxTokens": 8192},
				},
			},
			fixtureOther: map[string]any{
				"baseUrl": "https://elsewhere.example.com:443/v1",
				"models":  []any{map[string]any{"id": "elsewhere-model"}},
			},
		},
	}
}

func writeFixture(t *testing.T, source string, auth, models any) {
	t.Helper()
	for name, value := range map[string]any{piAuthFile: auth, piModelsFile: models} {
		src, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), src, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureRequest(t *testing.T) PiRequest {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	box := filepath.Join(root, "box")
	private := filepath.Join(root, "private")
	for _, dir := range []string{source, box, private} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, source, fixtureAuth(), fixtureModels())
	return PiRequest{
		Source:   source,
		Dir:      filepath.Join(private, "pi"),
		Box:      box,
		Provider: fixtureProvider,
		Model:    fixtureModel,
	}
}

func readProjected(t *testing.T, dir, name string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestPiProjectionCarriesOneProviderOneModelAndOnlyACredentialReference(t *testing.T) {
	req := fixtureRequest(t)
	state, err := ProjectPi(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Remove() })

	info, err := os.Stat(state.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("projection mode = %v, want owner-only", info.Mode().Perm())
	}

	auth := readProjected(t, state.Dir(), piAuthFile)
	var credentials map[string]piStoredCredential
	if err := json.Unmarshal(auth, &credentials); err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 1 || credentials[fixtureProvider].Type != "api_key" {
		t.Fatalf("projected credentials = %#v, want only the selected api_key entry", credentials)
	}
	if credentials[fixtureProvider].Key != piCredentialReference {
		t.Fatalf("projected key = %q, want the %s reference", credentials[fixtureProvider].Key, PiCredentialEnvironment)
	}

	models := readProjected(t, state.Dir(), piModelsFile)
	var file struct {
		Providers map[string]piProviderConfig `json:"providers"`
	}
	if err := json.Unmarshal(models, &file); err != nil {
		t.Fatal(err)
	}
	provider, found := file.Providers[fixtureProvider]
	if !found || len(file.Providers) != 1 {
		t.Fatalf("projected providers = %#v, want only the selected one", file.Providers)
	}
	if provider.BaseURL != "http://localhost:1234/v1" || provider.API != "openai-completions" {
		t.Fatalf("projected provider lost its request shape: %#v", provider)
	}
	if len(provider.Models) != 1 || len(provider.ModelOverrides) != 1 {
		t.Fatalf("projected provider = %#v, want exactly the selected model and its override", provider)
	}
	var definition piModelConfig
	if err := json.Unmarshal(provider.Models[0], &definition); err != nil {
		t.Fatal(err)
	}
	if definition.ID != fixtureModel {
		t.Fatalf("projected model = %q, want %q", definition.ID, fixtureModel)
	}
	if _, found := provider.ModelOverrides[fixtureModel]; !found {
		t.Fatalf("projected overrides = %#v, want the selected model's", provider.ModelOverrides)
	}

	settings := readProjected(t, state.Dir(), piSettingsFile)
	var projected map[string]any
	if err := json.Unmarshal(settings, &projected); err != nil {
		t.Fatal(err)
	}
	if projected["defaultProvider"] != fixtureProvider || projected["defaultModel"] != fixtureModel {
		t.Fatalf("projected settings = %#v, want the explicit selection", projected)
	}
	if projected["defaultProjectTrust"] != "never" {
		t.Fatalf("projected settings would let the repair tree become configuration: %#v", projected)
	}

	// Nothing the operator did not select, and no credential, survived the copy.
	for _, name := range []string{piAuthFile, piModelsFile, piSettingsFile} {
		src := string(readProjected(t, state.Dir(), name))
		for _, forbidden := range []string{fixtureKey, fixtureOther, "workshop-large", "elsewhere.example.com"} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("projected %s carried %q: %s", name, forbidden, src)
			}
		}
	}
	entries, err := os.ReadDir(state.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("projection holds %d entries, want exactly the three it is responsible for", len(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("projected %s mode = %v, want owner-only", entry.Name(), info.Mode().Perm())
		}
	}
}

// The projected directory is where Pi takes its proper-lockfile locks, because
// both HOME and PI_CODING_AGENT_DIR name it. Locks are directories created
// beside the file they guard, so this is the shape a read-only projection
// cannot satisfy; see the adapter test that drives the real environment.
func TestPiProjectionAcceptsTheLockDirectoriesPiTakesToRead(t *testing.T) {
	state, err := ProjectPi(fixtureRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Remove() })
	for _, name := range []string{piAuthFile, piModelsFile, piSettingsFile, "models-store.json", "trust.json"} {
		lock := filepath.Join(state.Dir(), name+".lock")
		if err := os.Mkdir(lock, 0o700); err != nil {
			t.Fatalf("Pi could not take its %s lock inside the projection: %v", name, err)
		}
	}
}

func TestPiProjectionIsDiscardedAndDiscardsIdempotently(t *testing.T) {
	state, err := ProjectPi(fixtureRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := state.Dir()
	if err := state.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("discarded projection is still at %s: %v", dir, err)
	}
	if err := state.Remove(); err != nil {
		t.Fatalf("second discard = %v, want it to be idempotent", err)
	}
	if (*PiState)(nil).Dir() != "" {
		t.Fatal("an unmaterialized projection reported a directory")
	}
	if err := (*PiState)(nil).Remove(); err != nil {
		t.Fatalf("discarding an unmaterialized projection = %v", err)
	}
}

// An unattended repair that names no endpoint gets loopback, and gets only
// loopback: the operator's own network is authorized one address at a time.
func TestPiProjectionDefaultsToLoopbackAndAuthorizesNothingElse(t *testing.T) {
	req := fixtureRequest(t)
	state, err := ProjectPi(req)
	if err != nil {
		t.Fatalf("the default endpoint was refused: %v", err)
	}
	if err := state.Remove(); err != nil {
		t.Fatal(err)
	}

	private := fixtureModels()
	providers := private["providers"].(map[string]any)
	providers[fixtureProvider].(map[string]any)["baseUrl"] = "http://10.1.2.3:11434/v1"
	writeFixture(t, req.Source, fixtureAuth(), private)

	if _, err := ProjectPi(req); err == nil || !strings.Contains(err.Error(), "not an authorized repair endpoint") {
		t.Fatalf("an unauthorized private endpoint = %v, want a refusal", err)
	}
	req.Endpoints = []string{"10.1.2.3:11434"}
	state, err = ProjectPi(req)
	if err != nil {
		t.Fatalf("an explicitly authorized private endpoint = %v", err)
	}
	if err := state.Remove(); err != nil {
		t.Fatal(err)
	}
}

// The hosts an operator's Pi configuration can name include a hosted provider,
// and a hosted provider is not self-hosted whatever the authorization list
// says. The rule is structural so it cannot be configured away by accident.
func TestPiProjectionRefusesAHostedEndpointEvenWhenItIsConfigured(t *testing.T) {
	for _, authority := range []string{
		"resource.openai.azure.com:443",
		"api.example.com:443",
		"8.8.8.8:443",
		"[2001:4860:4860::8888]:443",
	} {
		t.Run(authority, func(t *testing.T) {
			req := fixtureRequest(t)
			req.Endpoints = []string{authority}
			if _, err := ProjectPi(req); err == nil || !strings.Contains(err.Error(), "pi endpoint host") {
				t.Fatalf("authorizing %s = %v, want a refusal", authority, err)
			}
			hosted := fixtureModels()
			providers := hosted["providers"].(map[string]any)
			providers[fixtureProvider].(map[string]any)["baseUrl"] = "https://" + authority + "/v1"
			writeFixture(t, req.Source, fixtureAuth(), hosted)
			req.Endpoints = nil
			if _, err := ProjectPi(req); err == nil || !strings.Contains(err.Error(), "pi provider") {
				t.Fatalf("projecting %s = %v, want a refusal", authority, err)
			}
		})
	}
}

func TestPiProjectionRefusesEveryUnsafeSourceAndPlacement(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, req *PiRequest)
		want string
	}{
		{"no source", func(_ *testing.T, req *PiRequest) { req.Source = "" }, "source is unset"},
		{"relative source", func(_ *testing.T, req *PiRequest) { req.Source = "source" }, "source is not an explicit absolute path"},
		{"missing source", func(_ *testing.T, req *PiRequest) { req.Source = filepath.Join(req.Source, "absent") }, "resolve pi projection source"},
		{"source is a file", func(t *testing.T, req *PiRequest) {
			req.Source = filepath.Join(req.Source, piAuthFile)
		}, "source is not a directory"},
		{"no destination", func(_ *testing.T, req *PiRequest) { req.Dir = "" }, "destination is unset"},
		{"relative destination", func(_ *testing.T, req *PiRequest) { req.Dir = "pi" }, "destination is not an explicit absolute path"},
		{"destination exists", func(t *testing.T, req *PiRequest) {
			if err := os.Mkdir(req.Dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "destination already exists"},
		{"no box", func(_ *testing.T, req *PiRequest) { req.Box = "" }, "box is unset"},
		{"source inside the box", func(t *testing.T, req *PiRequest) {
			inside := filepath.Join(req.Box, "pi")
			if err := os.MkdirAll(inside, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, inside, fixtureAuth(), fixtureModels())
			req.Source = inside
		}, "source is inside the box"},
		{"destination inside the box", func(_ *testing.T, req *PiRequest) {
			req.Dir = filepath.Join(req.Box, "pi")
		}, "destination is inside the box"},
		{"destination inside the source", func(_ *testing.T, req *PiRequest) {
			req.Dir = filepath.Join(req.Source, "pi")
		}, "source and destination overlap"},
		{"destination is the source", func(_ *testing.T, req *PiRequest) {
			req.Dir = req.Source
		}, "source and destination overlap"},
		{"source inside the destination", func(_ *testing.T, req *PiRequest) {
			req.Dir = filepath.Dir(req.Source)
		}, "source and destination overlap"},
		{"no provider", func(_ *testing.T, req *PiRequest) { req.Provider = "" }, "provider is unset"},
		{"provider is an option", func(_ *testing.T, req *PiRequest) { req.Provider = "--no-tools" }, "provider is not a direct value"},
		{"no model", func(_ *testing.T, req *PiRequest) { req.Model = "" }, "model is unset"},
		{"model is an option", func(_ *testing.T, req *PiRequest) { req.Model = "--no-tools" }, "model is not a direct value"},

		{"symlinked credential file", func(t *testing.T, req *PiRequest) {
			elsewhere := filepath.Join(t.TempDir(), "operator-auth.json")
			if err := os.WriteFile(elsewhere, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(req.Source, piAuthFile)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, path); err != nil {
				t.Fatal(err)
			}
		}, "auth.json is a symlink"},
		{"models file is not a regular file", func(t *testing.T, req *PiRequest) {
			path := filepath.Join(req.Source, piModelsFile)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "models.json is not a regular file"},

		{"oauth credential", func(t *testing.T, req *PiRequest) {
			auth := fixtureAuth()
			auth[fixtureProvider] = map[string]any{"type": "oauth", "refresh": "r", "access": "a", "expires": 1}
			writeFixture(t, req.Source, auth, fixtureModels())
		}, "refuses the oauth credential"},
		{"unknown credential type", func(t *testing.T, req *PiRequest) {
			auth := fixtureAuth()
			auth[fixtureProvider] = map[string]any{"type": "device_code"}
			writeFixture(t, req.Source, auth, fixtureModels())
		}, "does not know credential type"},
		{"no credential for the provider", func(t *testing.T, req *PiRequest) {
			auth := fixtureAuth()
			delete(auth, fixtureProvider)
			writeFixture(t, req.Source, auth, fixtureModels())
		}, "holds no credential for provider"},
		{"credential shadows the channel", func(t *testing.T, req *PiRequest) {
			auth := fixtureAuth()
			auth[fixtureProvider] = map[string]any{
				"type": "api_key",
				"key":  "$" + PiCredentialEnvironment,
				"env":  map[string]any{PiCredentialEnvironment: "fixture-file-side-value"},
			}
			writeFixture(t, req.Source, auth, fixtureModels())
		}, "shadows " + PiCredentialEnvironment},

		{"provider authenticates with oauth", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["oauth"] = "radius"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "it authenticates with oauth"},
		{"provider declares an inline key", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["apiKey"] = "fixture-inline-value"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "declares an inline apiKey"},
		{"provider names no endpoint", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			delete(models["providers"].(map[string]any)[fixtureProvider].(map[string]any), "baseUrl")
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "names no baseUrl to authorize"},
		{"provider is absent", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			delete(models["providers"].(map[string]any), fixtureProvider)
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "defines no provider"},
		{"endpoint has no port", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["baseUrl"] = "http://localhost/v1"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "names no explicit port"},
		{"endpoint is not http", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["baseUrl"] = "file://localhost:1234/v1"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "neither http nor https"},
		{"endpoint carries credentials", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["baseUrl"] = "http://user:secret@localhost:1234/v1"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "carries credentials in its URL"},
		{"provider header is a shell command", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["headers"] = map[string]any{
				"X-Workshop": "!security find-generic-password -w",
			}
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "is a shell command"},
		{"provider header reads the environment", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["headers"] = map[string]any{
				"X-Workshop": "Bearer ${" + PiCredentialEnvironment + "}",
			}
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "reads a child environment value"},

		{"provider declares no models", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			delete(models["providers"].(map[string]any)[fixtureProvider].(map[string]any), "models")
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "it declares no models"},
		{"model is absent", func(_ *testing.T, req *PiRequest) { req.Model = "workshop-absent" }, "defines no model"},
		{"model overrides the endpoint", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			provider := models["providers"].(map[string]any)[fixtureProvider].(map[string]any)
			provider["models"] = []any{map[string]any{"id": fixtureModel, "baseUrl": "https://api.example.com:443/v1"}}
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "pi model " + fixtureModel + " baseUrl"},
		{"model header is a shell command", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			provider := models["providers"].(map[string]any)[fixtureProvider].(map[string]any)
			provider["models"] = []any{map[string]any{"id": fixtureModel, "headers": map[string]any{"X": "!id"}}}
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "pi model " + fixtureModel + " header"},
		{"model override header is a shell command", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			provider := models["providers"].(map[string]any)[fixtureProvider].(map[string]any)
			provider["modelOverrides"] = map[string]any{fixtureModel: map[string]any{"headers": map[string]any{"X": "!id"}}}
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "pi model override " + fixtureModel + " header"},

		{"provider carries a field the projection does not model", func(t *testing.T, req *PiRequest) {
			models := fixtureModels()
			models["providers"].(map[string]any)[fixtureProvider].(map[string]any)["authUrl"] = "http://localhost:1234/authorize"
			writeFixture(t, req.Source, fixtureAuth(), models)
		}, "decode pi projection source provider"},
		{"credential carries a field the projection does not model", func(t *testing.T, req *PiRequest) {
			auth := fixtureAuth()
			auth[fixtureProvider] = map[string]any{"type": "api_key", "key": fixtureKey, "device": "operator-laptop"}
			writeFixture(t, req.Source, auth, fixtureModels())
		}, "decode pi projection source credential"},
		{"credential file is not JSON", func(t *testing.T, req *PiRequest) {
			if err := os.WriteFile(filepath.Join(req.Source, piAuthFile), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "decode pi projection source auth.json"},
		{"models file is not JSON", func(t *testing.T, req *PiRequest) {
			if err := os.WriteFile(filepath.Join(req.Source, piModelsFile), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "decode pi projection source models.json"},
		{"endpoint configuration is not host:port", func(_ *testing.T, req *PiRequest) {
			req.Endpoints = []string{"localhost"}
		}, "is not host:port"},
		{"endpoint configuration has no valid port", func(_ *testing.T, req *PiRequest) {
			req.Endpoints = []string{"localhost:0"}
		}, "has no valid port"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := fixtureRequest(t)
			tc.edit(t, &req)
			_, occupied := os.Lstat(req.Dir)
			state, err := ProjectPi(req)
			if err == nil {
				_ = state.Remove()
				t.Fatalf("ProjectPi(%+v) succeeded, want refusal %q", req, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ProjectPi = %v, want refusal %q", err, tc.want)
			}
			// A refusal decides before the destination is created, so a run
			// that refuses leaves nothing for the next one to inherit or read.
			if occupied != nil {
				if _, err := os.Lstat(req.Dir); !os.IsNotExist(err) {
					t.Fatalf("a refused projection left %s behind: %v", req.Dir, err)
				}
			}
		})
	}
}

// Pi's own parse decides whether a configuration value is itself or a
// reference, and kept's has to agree with it or a reviewed literal resolves to
// something nobody reviewed.
func TestPiEnvironmentReferenceMatchesPisOwnParse(t *testing.T) {
	for value, want := range map[string]bool{
		"sk-literal":          false,
		"":                    false,
		"$":                   false,
		"$$literal":           false,
		"$!literal":           false,
		"$$":                  false,
		"a$1b":                false,
		"${1BAD}":             false,
		"${unterminated":      false,
		"$NAME":               true,
		"prefix-$NAME":        true,
		"${NAME}":             true,
		"a ${NAME} b":         true,
		"$$escaped-then-$VAR": true,
		"${_x9}":              true,
	} {
		if got := piEnvironmentReference(value); got != want {
			t.Fatalf("piEnvironmentReference(%q) = %v, want %v", value, got, want)
		}
	}
}
