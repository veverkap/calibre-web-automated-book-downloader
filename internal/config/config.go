package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Database
	CWADBPath string

	// Paths
	LogRoot   string
	LogDir    string
	TmpDir    string
	IngestDir string

	// Timeouts and retries
	StatusTimeout int
	MaxRetry      int
	DefaultSleep  int

	// Feature flags
	UseBookTitle  bool
	UseCFBypass   bool
	PrioritizeWELIB bool
	AllowUseWELIB bool
	Debug         bool
	EnableLogging bool
	DockerMode    bool
	UseDOH        bool
	UsingExternalBypasser bool
	UsingTor      bool

	// Proxies
	HTTPProxy  string
	HTTPSProxy string

	// Anna's Archive
	AADonatorKey      string
	AABaseURL         string
	AAAdditionalURLs  string

	// Book settings
	SupportedFormats string
	BookLanguage     string
	CustomScript     string

	// Server settings
	FlaskHost string
	FlaskPort int
	AppEnv    string
	LogLevel  string

	// Version
	BuildVersion   string
	ReleaseVersion string

	// Download settings
	MainLoopSleepTime              int
	MaxConcurrentDownloads         int
	DownloadProgressUpdateInterval int

	// DNS settings
	CustomDNS string

	// Cloudflare bypass settings
	BypassReleaseInactiveMin int

	// External bypasser settings
	ExtBypasserURL     string
	ExtBypasserPath    string
	ExtBypasserTimeout int
}

// Load loads configuration from environment variables using viper
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	cfg := &Config{
		CWADBPath:                      v.GetString("CWA_DB_PATH"),
		LogRoot:                        v.GetString("LOG_ROOT"),
		LogDir:                         v.GetString("LOG_DIR"),
		TmpDir:                         v.GetString("TMP_DIR"),
		IngestDir:                      v.GetString("INGEST_DIR"),
		StatusTimeout:                  v.GetInt("STATUS_TIMEOUT"),
		UseBookTitle:                   v.GetBool("USE_BOOK_TITLE"),
		MaxRetry:                       v.GetInt("MAX_RETRY"),
		DefaultSleep:                   v.GetInt("DEFAULT_SLEEP"),
		UseCFBypass:                    v.GetBool("USE_CF_BYPASS"),
		HTTPProxy:                      strings.TrimSpace(v.GetString("HTTP_PROXY")),
		HTTPSProxy:                     strings.TrimSpace(v.GetString("HTTPS_PROXY")),
		AADonatorKey:                   strings.TrimSpace(v.GetString("AA_DONATOR_KEY")),
		AABaseURL:                      strings.TrimSpace(v.GetString("AA_BASE_URL")),
		AAAdditionalURLs:               strings.TrimSpace(v.GetString("AA_ADDITIONAL_URLS")),
		SupportedFormats:               strings.ToLower(v.GetString("SUPPORTED_FORMATS")),
		BookLanguage:                   strings.ToLower(v.GetString("BOOK_LANGUAGE")),
		CustomScript:                   strings.TrimSpace(v.GetString("CUSTOM_SCRIPT")),
		FlaskHost:                      v.GetString("FLASK_HOST"),
		FlaskPort:                      v.GetInt("FLASK_PORT"),
		Debug:                          v.GetBool("DEBUG"),
		AppEnv:                         strings.ToLower(v.GetString("APP_ENV")),
		PrioritizeWELIB:                v.GetBool("PRIORITIZE_WELIB"),
		AllowUseWELIB:                  v.GetBool("ALLOW_USE_WELIB"),
		BuildVersion:                   v.GetString("BUILD_VERSION"),
		ReleaseVersion:                 v.GetString("RELEASE_VERSION"),
		EnableLogging:                  v.GetBool("ENABLE_LOGGING"),
		MainLoopSleepTime:              v.GetInt("MAIN_LOOP_SLEEP_TIME"),
		MaxConcurrentDownloads:         v.GetInt("MAX_CONCURRENT_DOWNLOADS"),
		DownloadProgressUpdateInterval: v.GetInt("DOWNLOAD_PROGRESS_UPDATE_INTERVAL"),
		DockerMode:                     v.GetBool("DOCKERMODE"),
		CustomDNS:                      strings.TrimSpace(v.GetString("CUSTOM_DNS")),
		UseDOH:                         v.GetBool("USE_DOH"),
		BypassReleaseInactiveMin:       v.GetInt("BYPASS_RELEASE_INACTIVE_MIN"),
		UsingExternalBypasser:          v.GetBool("USING_EXTERNAL_BYPASSER"),
		ExtBypasserURL:                 strings.TrimSpace(v.GetString("EXT_BYPASSER_URL")),
		ExtBypasserPath:                strings.TrimSpace(v.GetString("EXT_BYPASSER_PATH")),
		ExtBypasserTimeout:             v.GetInt("EXT_BYPASSER_TIMEOUT"),
		UsingTor:                       v.GetBool("USING_TOR"),
	}

	// Override log level if debug is enabled
	if cfg.Debug {
		cfg.LogLevel = "DEBUG"
	} else {
		cfg.LogLevel = strings.ToUpper(v.GetString("LOG_LEVEL"))
	}

	// If using Tor, override some settings
	if cfg.UsingTor {
		cfg.CustomDNS = ""
		cfg.UseDOH = false
		cfg.HTTPProxy = ""
		cfg.HTTPSProxy = ""
	}

	// Create log directory path
	if cfg.LogRoot == "" {
		cfg.LogRoot = "/var/log/"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = cfg.LogRoot + "/cwa-book-downloader"
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("LOG_ROOT", "/var/log/")
	v.SetDefault("TMP_DIR", "/tmp/cwa-book-downloader")
	v.SetDefault("INGEST_DIR", "/cwa-book-ingest")
	v.SetDefault("STATUS_TIMEOUT", 3600)
	v.SetDefault("USE_BOOK_TITLE", false)
	v.SetDefault("MAX_RETRY", 10)
	v.SetDefault("DEFAULT_SLEEP", 5)
	v.SetDefault("USE_CF_BYPASS", true)
	v.SetDefault("AA_BASE_URL", "auto")
	v.SetDefault("SUPPORTED_FORMATS", "epub,mobi,azw3,fb2,djvu,cbz,cbr")
	v.SetDefault("BOOK_LANGUAGE", "en")
	v.SetDefault("FLASK_HOST", "0.0.0.0")
	v.SetDefault("FLASK_PORT", 8084)
	v.SetDefault("DEBUG", false)
	v.SetDefault("APP_ENV", "N/A")
	v.SetDefault("PRIORITIZE_WELIB", false)
	v.SetDefault("ALLOW_USE_WELIB", true)
	v.SetDefault("BUILD_VERSION", "N/A")
	v.SetDefault("RELEASE_VERSION", "N/A")
	v.SetDefault("LOG_LEVEL", "INFO")
	v.SetDefault("ENABLE_LOGGING", true)
	v.SetDefault("MAIN_LOOP_SLEEP_TIME", 5)
	v.SetDefault("MAX_CONCURRENT_DOWNLOADS", 3)
	v.SetDefault("DOWNLOAD_PROGRESS_UPDATE_INTERVAL", 5)
	v.SetDefault("DOCKERMODE", false)
	v.SetDefault("USE_DOH", false)
	v.SetDefault("BYPASS_RELEASE_INACTIVE_MIN", 5)
	v.SetDefault("USING_EXTERNAL_BYPASSER", false)
	v.SetDefault("EXT_BYPASSER_URL", "http://flaresolverr:8191")
	v.SetDefault("EXT_BYPASSER_PATH", "/v1")
	v.SetDefault("EXT_BYPASSER_TIMEOUT", 60000)
	v.SetDefault("USING_TOR", false)
}

// stringToBool converts a string to a boolean
// Accepts: "true", "yes", "1", "y" (case insensitive)
func stringToBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "yes" || s == "1" || s == "y"
}

// GetBool gets a boolean value from an environment variable
func GetBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return stringToBool(value)
}

// GetInt gets an integer value from an environment variable
func GetInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}
