package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidCompleteConfiguration(t *testing.T) {
	config, err := load(missingDotEnvPath(t), mapLookup(validValues()))
	if err != nil {
		t.Fatalf("load valid configuration: %v", err)
	}

	if config.Environment != EnvironmentProduction {
		t.Errorf("Environment = %q, want %q", config.Environment, EnvironmentProduction)
	}
	if got := config.API.Bind.String(); got != "0.0.0.0" {
		t.Errorf("API.Bind = %q, want %q", got, "0.0.0.0")
	}
	if config.API.Port != 9090 {
		t.Errorf("API.Port = %d, want 9090", config.API.Port)
	}
	if config.Database.Host != "postgres" || config.Database.Port != 5544 {
		t.Errorf("Database host/port = %s/%d, want postgres/5544", config.Database.Host, config.Database.Port)
	}
	if config.Database.Name != "lifeos_test" || config.Database.User != "lifeos_test" {
		t.Errorf("Database name/user = %s/%s, want lifeos_test/lifeos_test", config.Database.Name, config.Database.User)
	}
	if config.Database.Password != "valid-development-secret" {
		t.Error("Database.Password did not retain the resolved value")
	}
	if config.Logging.Level != LogLevelDebug {
		t.Errorf("Logging.Level = %q, want %q", config.Logging.Level, LogLevelDebug)
	}
	if config.Web.APIBaseURL != "https://example.test/api" {
		t.Errorf("Web.APIBaseURL = %q, want %q", config.Web.APIBaseURL, "https://example.test/api")
	}
}

func TestLoadUsesDocumentedDefaults(t *testing.T) {
	config, err := load(missingDotEnvPath(t), mapLookup(map[string]string{
		keyDBPassword: "valid-development-secret",
	}))
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if config.Environment != EnvironmentDevelopment {
		t.Errorf("Environment = %q, want %q", config.Environment, EnvironmentDevelopment)
	}
	if got := config.API.Bind.String(); got != "127.0.0.1" {
		t.Errorf("API.Bind = %q, want 127.0.0.1", got)
	}
	if config.API.Port != 8080 {
		t.Errorf("API.Port = %d, want 8080", config.API.Port)
	}
	if config.Database.Host != "127.0.0.1" || config.Database.Port != 5432 {
		t.Errorf("Database host/port = %s/%d, want 127.0.0.1/5432", config.Database.Host, config.Database.Port)
	}
	if config.Database.Name != "lifeos" || config.Database.User != "lifeos" {
		t.Errorf("Database name/user = %s/%s, want lifeos/lifeos", config.Database.Name, config.Database.User)
	}
	if config.Logging.Level != LogLevelInfo {
		t.Errorf("Logging.Level = %q, want %q", config.Logging.Level, LogLevelInfo)
	}
	if config.Web.APIBaseURL != "http://127.0.0.1:8080" {
		t.Errorf("Web.APIBaseURL = %q, want default local URL", config.Web.APIBaseURL)
	}
}

func TestProcessEnvironmentOverridesDotEnv(t *testing.T) {
	dotEnvPath := writeDotEnv(t, strings.Join([]string{
		"LIFEOS_ENV=test",
		"LIFEOS_API_PORT=8081",
		"LIFEOS_DB_PASSWORD=file-secret",
		"LIFEOS_LOG_LEVEL=warn",
	}, "\n"))

	config, err := load(dotEnvPath, mapLookup(map[string]string{
		keyEnvironment: "production",
		keyAPIPort:     "9090",
		keyDBPassword:  "process-secret",
	}))
	if err != nil {
		t.Fatalf("load precedence configuration: %v", err)
	}

	if config.Environment != EnvironmentProduction || config.API.Port != 9090 {
		t.Errorf("process values did not override .env: environment=%q port=%d", config.Environment, config.API.Port)
	}
	if config.Database.Password != "process-secret" {
		t.Error("process password did not override .env password")
	}
	if config.Logging.Level != LogLevelWarn {
		t.Errorf("non-overridden .env log level = %q, want %q", config.Logging.Level, LogLevelWarn)
	}
}

func TestDotEnvOverridesDefaults(t *testing.T) {
	dotEnvPath := writeDotEnv(t, strings.Join([]string{
		"LIFEOS_ENV=test",
		"LIFEOS_API_PORT=8081",
		"LIFEOS_DB_HOST=postgres",
		"LIFEOS_DB_PORT=6543",
		"LIFEOS_DB_PASSWORD=file-secret",
		"VITE_API_BASE_URL=https://localhost.test:8081",
	}, "\n"))

	config, err := load(dotEnvPath, mapLookup(nil))
	if err != nil {
		t.Fatalf("load .env configuration: %v", err)
	}

	if config.Environment != EnvironmentTest || config.API.Port != 8081 {
		t.Errorf(".env did not override defaults: environment=%q port=%d", config.Environment, config.API.Port)
	}
	if config.Database.Host != "postgres" || config.Database.Port != 6543 {
		t.Errorf(".env database host/port = %s/%d, want postgres/6543", config.Database.Host, config.Database.Port)
	}
	if config.Web.APIBaseURL != "https://localhost.test:8081" {
		t.Errorf(".env web URL = %q, want override", config.Web.APIBaseURL)
	}
}

func TestProjectDotEnvPathFromModuleRoot(t *testing.T) {
	root := writeModuleRoot(t, "github.com/Faust217/LifeOS")
	want := filepath.Join(root, ".env")

	got, ok := projectDotEnvPath(root)
	if !ok {
		t.Fatal("projectDotEnvPath did not find the LifeOS module root")
	}
	if got != want {
		t.Errorf("projectDotEnvPath = %q, want %q", got, want)
	}
}

func TestProjectDotEnvPathFromNestedDirectory(t *testing.T) {
	root := writeModuleRoot(t, "github.com/Faust217/LifeOS")
	nested := filepath.Join(root, "internal", "platform", "config")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	want := filepath.Join(root, ".env")
	got, ok := projectDotEnvPath(nested)
	if !ok {
		t.Fatal("projectDotEnvPath did not find the ancestor LifeOS module root")
	}
	if got != want {
		t.Errorf("projectDotEnvPath = %q, want %q", got, want)
	}
}

func TestProjectDotEnvPathDoesNotUseUnrelatedModule(t *testing.T) {
	root := writeModuleRoot(t, "example.com/unrelated")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	got, ok := projectDotEnvPath(nested)
	if ok {
		t.Fatalf("projectDotEnvPath selected unrelated .env %q", got)
	}
	if got != "" {
		t.Errorf("projectDotEnvPath = %q without a match, want empty path", got)
	}
}

func TestEmptyDotEnvValueOverridesDefaultAndFailsValidation(t *testing.T) {
	dotEnvPath := writeDotEnv(t, strings.Join([]string{
		"LIFEOS_API_PORT=",
		"LIFEOS_DB_PASSWORD=valid-development-secret",
	}, "\n"))

	_, err := load(dotEnvPath, mapLookup(nil))
	requireErrorForKey(t, err, keyAPIPort)
}

func TestLoadRejectsMissingDatabasePassword(t *testing.T) {
	_, err := load(missingDotEnvPath(t), mapLookup(nil))
	requireErrorForKey(t, err, keyDBPassword)
}

func TestLoadRejectsEmptyProcessPasswordOverDotEnv(t *testing.T) {
	dotEnvPath := writeDotEnv(t, "LIFEOS_DB_PASSWORD=file-secret\n")
	_, err := load(dotEnvPath, mapLookup(map[string]string{keyDBPassword: ""}))
	requireErrorForKey(t, err, keyDBPassword)
}

func TestLoadRejectsPasswordPlaceholder(t *testing.T) {
	values := validValues()
	values[keyDBPassword] = "CHANGE_ME"
	_, err := load(missingDotEnvPath(t), mapLookup(values))
	requireErrorForKey(t, err, keyDBPassword)
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	values := validValues()
	values[keyEnvironment] = "staging"
	_, err := load(missingDotEnvPath(t), mapLookup(values))
	requireErrorForKey(t, err, keyEnvironment)
}

func TestLoadRejectsInvalidAPIPorts(t *testing.T) {
	testCases := map[string]string{
		"malformed":   "eight-thousand",
		"below range": "0",
		"above range": "65536",
	}
	for name, value := range testCases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values[keyAPIPort] = value
			_, err := load(missingDotEnvPath(t), mapLookup(values))
			requireErrorForKey(t, err, keyAPIPort)
		})
	}
}

func TestLoadRejectsInvalidDatabasePorts(t *testing.T) {
	testCases := map[string]string{
		"malformed":   "postgres",
		"below range": "0",
		"above range": "65536",
	}
	for name, value := range testCases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values[keyDBPort] = value
			_, err := load(missingDotEnvPath(t), mapLookup(values))
			requireErrorForKey(t, err, keyDBPort)
		})
	}
}

func TestLoadRejectsInvalidWebAPIURLs(t *testing.T) {
	testCases := map[string]string{
		"relative":           "/api",
		"wrong scheme":       "ftp://example.test/api",
		"missing host":       "http:///api",
		"embedded user info": "http://user:password@example.test/api",
	}
	for name, value := range testCases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values[keyWebAPIURL] = value
			_, err := load(missingDotEnvPath(t), mapLookup(values))
			requireErrorForKey(t, err, keyWebAPIURL)
		})
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	values := validValues()
	values[keyLogLevel] = "trace"
	_, err := load(missingDotEnvPath(t), mapLookup(values))
	requireErrorForKey(t, err, keyLogLevel)
}

func TestLoadRejectsInvalidBindAddress(t *testing.T) {
	values := validValues()
	values[keyAPIBind] = "not a bind address"
	_, err := load(missingDotEnvPath(t), mapLookup(values))
	requireErrorForKey(t, err, keyAPIBind)
}

func TestLoadRejectsInvalidRequiredDatabaseValues(t *testing.T) {
	testCases := map[string]struct {
		key   string
		value string
	}{
		"host": {key: keyDBHost, value: "invalid host!"},
		"name": {key: keyDBName, value: "   "},
		"user": {key: keyDBUser, value: "   "},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			values := validValues()
			values[testCase.key] = testCase.value
			_, err := load(missingDotEnvPath(t), mapLookup(values))
			requireErrorForKey(t, err, testCase.key)
		})
	}
}

func TestReturnedErrorsNeverContainPassword(t *testing.T) {
	const secret = "do-not-echo-this-secret"
	values := validValues()
	values[keyDBPassword] = secret
	values[keyEnvironment] = "invalid"

	_, err := load(missingDotEnvPath(t), mapLookup(values))
	if err == nil {
		t.Fatal("load returned no error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed database password: %v", err)
	}
}

func TestDotEnvParseErrorsDoNotEchoValues(t *testing.T) {
	const secret = "do-not-echo-this-secret"
	dotEnvPath := writeDotEnv(t, "LIFEOS_DB_PASSWORD=\""+secret+"\n")

	_, err := load(dotEnvPath, mapLookup(nil))
	requireErrorForKey(t, err, keyDBPassword)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposed malformed .env value: %v", err)
	}
}

func validValues() map[string]string {
	return map[string]string{
		keyEnvironment: "production",
		keyAPIBind:     "0.0.0.0",
		keyAPIPort:     "9090",
		keyDBHost:      "postgres",
		keyDBPort:      "5544",
		keyDBName:      "lifeos_test",
		keyDBUser:      "lifeos_test",
		keyDBPassword:  "valid-development-secret",
		keyLogLevel:    "debug",
		keyWebAPIURL:   "https://example.test/api",
	}
}

func mapLookup(values map[string]string) lookupEnvironment {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func missingDotEnvPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".env")
}

func writeDotEnv(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write test .env: %v", err)
	}
	return path
}

func writeModuleRoot(t *testing.T, modulePath string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+modulePath+"\n"), 0o600); err != nil {
		t.Fatalf("write test go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("LIFEOS_DB_PASSWORD=test-secret\n"), 0o600); err != nil {
		t.Fatalf("write test .env: %v", err)
	}
	return root
}

func requireErrorForKey(t *testing.T, err error, key string) {
	t.Helper()
	if err == nil {
		t.Fatalf("load returned no error for %s", key)
	}
	if !strings.Contains(err.Error(), key) {
		t.Fatalf("error %q does not identify %s", err, key)
	}
}
