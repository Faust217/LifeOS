// Package config loads and validates the LifeOS Foundation runtime configuration.
package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	lifeOSModulePath = "github.com/Faust217/LifeOS"

	keyEnvironment = "LIFEOS_ENV"
	keyAPIBind     = "LIFEOS_API_BIND"
	keyAPIPort     = "LIFEOS_API_PORT"
	keyDBHost      = "LIFEOS_DB_HOST"
	keyDBPort      = "LIFEOS_DB_PORT"
	keyDBName      = "LIFEOS_DB_NAME"
	keyDBUser      = "LIFEOS_DB_USER"
	keyDBPassword  = "LIFEOS_DB_PASSWORD"
	keyLogLevel    = "LIFEOS_LOG_LEVEL"
	keyWebAPIURL   = "VITE_API_BASE_URL"
)

var foundationKeys = []string{
	keyEnvironment,
	keyAPIBind,
	keyAPIPort,
	keyDBHost,
	keyDBPort,
	keyDBName,
	keyDBUser,
	keyDBPassword,
	keyLogLevel,
	keyWebAPIURL,
}

// Environment identifies the application runtime mode.
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

// LogLevel controls the minimum application log severity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Config contains only the accepted Foundation runtime settings.
type Config struct {
	Environment Environment
	API         APIConfig
	Database    DatabaseConfig
	Logging     LoggingConfig
	Web         WebConfig
}

// APIConfig contains HTTP listener settings.
type APIConfig struct {
	Bind netip.Addr
	Port uint16
}

// DatabaseConfig contains PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     uint16
	Name     string
	User     string
	Password string
}

// LoggingConfig contains application logging settings.
type LoggingConfig struct {
	Level LogLevel
}

// WebConfig contains browser-visible web client settings.
type WebConfig struct {
	APIBaseURL string
}

type lookupEnvironment func(string) (string, bool)

// Load resolves process environment variables over the LifeOS module-root .env
// file and documented safe defaults, then validates the complete result.
func Load() (Config, error) {
	dotEnvPath := ""
	if workingDirectory, err := os.Getwd(); err == nil {
		dotEnvPath, _ = projectDotEnvPath(workingDirectory)
	}
	return load(dotEnvPath, os.LookupEnv)
}

func load(dotEnvPath string, lookup lookupEnvironment) (Config, error) {
	values := defaultValues()

	if dotEnvPath != "" {
		fileValues, err := readDotEnv(dotEnvPath)
		if err != nil {
			return Config{}, err
		}
		for _, key := range foundationKeys {
			if value, ok := fileValues[key]; ok {
				values[key] = value
			}
		}
	}

	for _, key := range foundationKeys {
		if value, ok := lookup(key); ok {
			values[key] = value
		}
	}

	return validate(values)
}

func projectDotEnvPath(startDirectory string) (string, bool) {
	directory, err := filepath.Abs(startDirectory)
	if err != nil {
		return "", false
	}

	for {
		if isLifeOSModuleRoot(directory) {
			return filepath.Join(directory, ".env"), true
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func isLifeOSModuleRoot(directory string) bool {
	contents, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "module" {
			return len(fields) == 2 && fields[1] == lifeOSModulePath
		}
	}
	return false
}

func defaultValues() map[string]string {
	return map[string]string{
		keyEnvironment: string(EnvironmentDevelopment),
		keyAPIBind:     "127.0.0.1",
		keyAPIPort:     "8080",
		keyDBHost:      "127.0.0.1",
		keyDBPort:      "5432",
		keyDBName:      "lifeos",
		keyDBUser:      "lifeos",
		keyLogLevel:    string(LogLevelInfo),
		keyWebAPIURL:   "http://127.0.0.1:8080",
	}
}

func validate(values map[string]string) (Config, error) {
	environment, err := parseEnvironment(values[keyEnvironment])
	if err != nil {
		return Config{}, err
	}

	bindValue := strings.TrimSpace(values[keyAPIBind])
	bind, err := netip.ParseAddr(bindValue)
	if err != nil {
		return Config{}, fmt.Errorf("%s must be a valid IP bind address", keyAPIBind)
	}

	apiPort, err := parsePort(keyAPIPort, values[keyAPIPort])
	if err != nil {
		return Config{}, err
	}

	dbHost := strings.TrimSpace(values[keyDBHost])
	if !isValidHost(dbHost) {
		return Config{}, fmt.Errorf("%s must be a valid host name or IP address", keyDBHost)
	}

	dbPort, err := parsePort(keyDBPort, values[keyDBPort])
	if err != nil {
		return Config{}, err
	}

	dbName := strings.TrimSpace(values[keyDBName])
	if dbName == "" {
		return Config{}, fmt.Errorf("%s must not be empty", keyDBName)
	}

	dbUser := strings.TrimSpace(values[keyDBUser])
	if dbUser == "" {
		return Config{}, fmt.Errorf("%s must not be empty", keyDBUser)
	}

	dbPassword := values[keyDBPassword]
	if strings.TrimSpace(dbPassword) == "" {
		return Config{}, fmt.Errorf("%s is required and must not be empty", keyDBPassword)
	}
	if strings.TrimSpace(dbPassword) == "CHANGE_ME" {
		return Config{}, fmt.Errorf("%s must be replaced with a real secret", keyDBPassword)
	}

	logLevel, err := parseLogLevel(values[keyLogLevel])
	if err != nil {
		return Config{}, err
	}

	webAPIURL := strings.TrimSpace(values[keyWebAPIURL])
	if err := validateWebAPIURL(webAPIURL); err != nil {
		return Config{}, err
	}

	return Config{
		Environment: environment,
		API: APIConfig{
			Bind: bind,
			Port: apiPort,
		},
		Database: DatabaseConfig{
			Host:     dbHost,
			Port:     dbPort,
			Name:     dbName,
			User:     dbUser,
			Password: dbPassword,
		},
		Logging: LoggingConfig{
			Level: logLevel,
		},
		Web: WebConfig{
			APIBaseURL: webAPIURL,
		},
	}, nil
}

func parseEnvironment(value string) (Environment, error) {
	environment := Environment(strings.TrimSpace(value))
	switch environment {
	case EnvironmentDevelopment, EnvironmentTest, EnvironmentProduction:
		return environment, nil
	default:
		return "", fmt.Errorf("%s must be one of development, test, or production", keyEnvironment)
	}
}

func parseLogLevel(value string) (LogLevel, error) {
	level := LogLevel(strings.TrimSpace(value))
	switch level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return level, nil
	default:
		return "", fmt.Errorf("%s must be one of debug, info, warn, or error", keyLogLevel)
	}
}

func parsePort(key, value string) (uint16, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be an integer from 1 through 65535", key)
	}
	return uint16(port), nil
}

func validateWebAPIURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL without credentials", keyWebAPIURL)
	}
	return nil
}

func isValidHost(value string) bool {
	if value == "" {
		return false
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return true
	}

	host := strings.TrimSuffix(value, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !isAlphaNumeric(label[0]) || !isAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			character := label[index]
			if !isAlphaNumeric(character) && character != '-' {
				return false
			}
		}
	}
	return true
}

func isAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
