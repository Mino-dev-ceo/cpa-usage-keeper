package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"github.com/joho/godotenv"
)

const (
	DefaultTimeZone               = "Asia/Shanghai"
	RedisQueueKeyDefault          = cpa.ManagementUsageQueueKey
	RedisQueueErrorBackoffDefault = 10 * time.Second
	MetadataSyncIntervalDefault   = 30 * time.Second
	AccountGuardIntervalDefault   = 5 * time.Minute
)

var (
	DefaultWorkDir      = filepath.Join(".", "data")
	DefaultSQLitePath   = filepath.Join(DefaultWorkDir, "app.db")
	DefaultLogDir       = filepath.Join(DefaultWorkDir, "logs")
	DefaultBackupDir    = filepath.Join(DefaultWorkDir, "backups")
	workDirDatabaseName = filepath.Base(DefaultSQLitePath)
	workDirLogsName     = filepath.Base(DefaultLogDir)
	workDirBackupsName  = filepath.Base(DefaultBackupDir)
)

type Config struct {
	// AppPort 是 Web 服务监听端口。
	AppPort string
	// AppBasePath 是 Web 服务部署子路径，空值表示根路径。
	AppBasePath string
	// CPABaseURL 是 CPA 服务基础地址。
	CPABaseURL string
	// CPAManagementKey 是访问 CPA 管理数据的密钥。
	CPAManagementKey string
	// RedisQueueAddr 是 CPA management data stream 的 TCP 地址，空值时按 CPA_BASE_URL 推导。
	RedisQueueAddr string
	// RedisQueueTLS 控制是否使用 TLS 连接 Redis 队列。
	RedisQueueTLS bool
	// RedisQueueKey 是 CPA usage 队列名。
	RedisQueueKey string
	// RedisQueueBatchSize 是单次 Redis LPOP 最多拉取的消息数。
	RedisQueueBatchSize int
	// RedisQueueIdleInterval 是 Redis 队列为空时的下一次检查间隔。
	RedisQueueIdleInterval time.Duration
	// RedisQueueErrorBackoff 是 Redis 临时错误后的固定退避间隔。
	RedisQueueErrorBackoff time.Duration
	// MetadataSyncInterval 是 auth files 和 provider metadata 的固定刷新间隔。
	MetadataSyncInterval time.Duration
	// WorkDir 是应用工作目录，数据库、日志和备份默认从这里派生。
	WorkDir string
	// SQLitePath 是 SQLite 数据库文件路径。
	SQLitePath string
	// BackupEnabled 控制是否保存 SQLite 数据库备份文件。
	BackupEnabled bool
	// BackupDir 是 SQLite 数据库备份目录。
	BackupDir string
	// BackupInterval 是两次备份写入之间的最小间隔。
	BackupInterval time.Duration
	// BackupRetentionDays 是备份文件保留天数。
	BackupRetentionDays int
	// RequestTimeout 是访问 CPA HTTP 和 Redis TCP 的超时时间。
	RequestTimeout time.Duration
	// TLSSkipVerify 控制是否跳过 CPA HTTPS 和 Redis 队列 TLS 的证书验证。
	TLSSkipVerify bool
	// LogLevel 是应用日志级别。
	LogLevel string
	// LogFileEnabled 控制是否写入持久化日志文件。
	LogFileEnabled bool
	// LogDir 是应用日志文件目录。
	LogDir string
	// LogRetentionDays 是日志保留天数，0 表示不自动清理。
	LogRetentionDays int
	// AuthEnabled 控制是否启用登录保护。
	AuthEnabled bool
	// LoginPassword 是启用登录保护时使用的登录密码。
	LoginPassword string
	// AuthSessionTTL 是登录 session 有效时长。
	AuthSessionTTL time.Duration
	// AccountGuardEnabled 控制是否启用按本地 usage 阈值自动禁用 auth file。
	AccountGuardEnabled bool
	// AccountGuardInterval 是账号守护任务的巡检间隔。
	AccountGuardInterval time.Duration
	// AccountGuardUsageThreshold 是本地周用量达到多少比例时触发禁用，例如 0.8。
	AccountGuardUsageThreshold float64
	// AccountGuardWeeklyTokenLimit 是单个 auth file 每周允许的总 token 基线。
	AccountGuardWeeklyTokenLimit int64
	// AccountGuardProviderQuotaEnabled 控制是否优先使用上游真实 quota 百分比。
	AccountGuardProviderQuotaEnabled bool
	// AccountGuardDryRun 为 true 时只记录日志和结果，不真正禁用账号。
	AccountGuardDryRun bool
	// AccountGuardAutoReenable 控制周窗口重置后是否自动重新启用由守护任务禁用的账号。
	AccountGuardAutoReenable bool
	// AccountGuardResetWeekday 是周窗口重置星期。
	AccountGuardResetWeekday time.Weekday
	// AccountGuardResetHour 是周窗口重置小时，使用项目本地时区。
	AccountGuardResetHour int
	// AccountGuardRemoveBannedEnabled 控制是否启用批量移除真正不可用账号。
	AccountGuardRemoveBannedEnabled bool
	// AccountGuardRemoveBannedDryRun 为 true 时只预览候选账号，不真正删除。
	AccountGuardRemoveBannedDryRun bool
	// AccountGuardRemoveBannedStatusMessages 是可判定为封禁/不可用的状态消息片段。
	AccountGuardRemoveBannedStatusMessages []string
}

type LoadOptions struct {
	EnvFile string
}

var executableDir = func() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executablePath), nil
}

func LoadFromEnv() (*Config, error) {
	return Load(LoadOptions{})
}

func Load(options LoadOptions) (*Config, error) {
	envBaseDir, err := loadDotEnv(options)
	if err != nil {
		return nil, err
	}
	if err := applyProjectTimeZone(); err != nil {
		return nil, err
	}

	redisQueueBatchSize, err := getInt("REDIS_QUEUE_BATCH_SIZE", 1000)
	if err != nil {
		return nil, err
	}
	if redisQueueBatchSize <= 0 {
		return nil, fmt.Errorf("REDIS_QUEUE_BATCH_SIZE must be positive")
	}

	redisQueueIdleInterval, err := getDuration("REDIS_QUEUE_IDLE_INTERVAL", time.Second)
	if err != nil {
		return nil, err
	}
	if redisQueueIdleInterval <= 0 {
		return nil, fmt.Errorf("REDIS_QUEUE_IDLE_INTERVAL must be positive")
	}

	requestTimeout, err := getDuration("REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}

	backupEnabled, err := getBool("BACKUP_ENABLED", true)
	if err != nil {
		return nil, err
	}

	backupInterval, err := getDuration("BACKUP_INTERVAL", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	if backupInterval <= 0 {
		return nil, fmt.Errorf("BACKUP_INTERVAL must be positive")
	}

	backupRetentionDays, err := getInt("BACKUP_RETENTION_DAYS", 7)
	if err != nil {
		return nil, err
	}
	if backupRetentionDays < 0 {
		return nil, fmt.Errorf("BACKUP_RETENTION_DAYS must be non-negative")
	}

	logFileEnabled, err := getBool("LOG_FILE_ENABLED", true)
	if err != nil {
		return nil, err
	}
	logRetentionDays, err := getInt("LOG_RETENTION_DAYS", 7)
	if err != nil {
		return nil, err
	}
	if logRetentionDays < 0 {
		return nil, fmt.Errorf("LOG_RETENTION_DAYS must be non-negative")
	}

	authSessionTTL, err := getDuration("AUTH_SESSION_TTL", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}
	if authSessionTTL <= 0 {
		return nil, fmt.Errorf("AUTH_SESSION_TTL must be positive")
	}

	authEnabled, err := getBool("AUTH_ENABLED", false)
	if err != nil {
		return nil, err
	}

	tlsSkipVerify, err := getBool("TLS_SKIP_VERIFY", false)
	if err != nil {
		return nil, err
	}

	redisQueueTLS, err := getBool("REDIS_QUEUE_TLS", false)
	if err != nil {
		return nil, err
	}

	accountGuardEnabled, err := getBool("ACCOUNT_GUARD_ENABLED", false)
	if err != nil {
		return nil, err
	}
	accountGuardInterval, err := getDuration("ACCOUNT_GUARD_INTERVAL", AccountGuardIntervalDefault)
	if err != nil {
		return nil, err
	}
	if accountGuardInterval <= 0 {
		return nil, fmt.Errorf("ACCOUNT_GUARD_INTERVAL must be positive")
	}
	accountGuardUsageThreshold, err := getFloat("ACCOUNT_GUARD_USAGE_THRESHOLD", 0.8)
	if err != nil {
		return nil, err
	}
	if accountGuardUsageThreshold <= 0 || accountGuardUsageThreshold > 1 {
		return nil, fmt.Errorf("ACCOUNT_GUARD_USAGE_THRESHOLD must be greater than 0 and less than or equal to 1")
	}
	accountGuardWeeklyTokenLimit, err := getInt64("ACCOUNT_GUARD_WEEKLY_TOKEN_LIMIT", 0)
	if err != nil {
		return nil, err
	}
	if accountGuardWeeklyTokenLimit < 0 {
		return nil, fmt.Errorf("ACCOUNT_GUARD_WEEKLY_TOKEN_LIMIT must be non-negative")
	}
	accountGuardProviderQuotaEnabled, err := getBool("ACCOUNT_GUARD_PROVIDER_QUOTA_ENABLED", true)
	if err != nil {
		return nil, err
	}
	accountGuardDryRun, err := getBool("ACCOUNT_GUARD_DRY_RUN", true)
	if err != nil {
		return nil, err
	}
	accountGuardAutoReenable, err := getBool("ACCOUNT_GUARD_AUTO_REENABLE", true)
	if err != nil {
		return nil, err
	}
	accountGuardResetWeekday, err := parseWeekday(getString("ACCOUNT_GUARD_RESET_WEEKDAY", "Monday"))
	if err != nil {
		return nil, fmt.Errorf("ACCOUNT_GUARD_RESET_WEEKDAY is invalid: %w", err)
	}
	accountGuardResetHour, err := getInt("ACCOUNT_GUARD_RESET_HOUR", 0)
	if err != nil {
		return nil, err
	}
	if accountGuardResetHour < 0 || accountGuardResetHour > 23 {
		return nil, fmt.Errorf("ACCOUNT_GUARD_RESET_HOUR must be between 0 and 23")
	}
	accountGuardRemoveBannedEnabled, err := getBool("ACCOUNT_GUARD_REMOVE_BANNED_ENABLED", false)
	if err != nil {
		return nil, err
	}
	accountGuardRemoveBannedDryRun, err := getBool("ACCOUNT_GUARD_REMOVE_BANNED_DRY_RUN", true)
	if err != nil {
		return nil, err
	}
	accountGuardRemoveBannedStatusMessages := getStringList("ACCOUNT_GUARD_REMOVE_BANNED_STATUS_MESSAGES", []string{"unauthorized", "payment_required", "not_found"})

	appBasePath, err := normalizeBasePath(strings.TrimSpace(os.Getenv("APP_BASE_PATH")))
	if err != nil {
		return nil, fmt.Errorf("APP_BASE_PATH is invalid: %w", err)
	}

	workDir := getString("WORK_DIR", DefaultWorkDir)

	cfg := &Config{
		AppPort:                                getString("APP_PORT", "8080"),
		AppBasePath:                            appBasePath,
		CPABaseURL:                             strings.TrimSpace(os.Getenv("CPA_BASE_URL")),
		CPAManagementKey:                       strings.TrimSpace(os.Getenv("CPA_MANAGEMENT_KEY")),
		RedisQueueAddr:                         strings.TrimSpace(os.Getenv("REDIS_QUEUE_ADDR")),
		RedisQueueTLS:                          redisQueueTLS,
		RedisQueueKey:                          RedisQueueKeyDefault,
		RedisQueueBatchSize:                    redisQueueBatchSize,
		RedisQueueIdleInterval:                 redisQueueIdleInterval,
		RedisQueueErrorBackoff:                 RedisQueueErrorBackoffDefault,
		MetadataSyncInterval:                   MetadataSyncIntervalDefault,
		WorkDir:                                workDir,
		SQLitePath:                             filepath.Join(workDir, workDirDatabaseName),
		BackupEnabled:                          backupEnabled,
		BackupDir:                              filepath.Join(workDir, workDirBackupsName),
		BackupInterval:                         backupInterval,
		BackupRetentionDays:                    backupRetentionDays,
		RequestTimeout:                         requestTimeout,
		TLSSkipVerify:                          tlsSkipVerify,
		LogLevel:                               getString("LOG_LEVEL", "info"),
		LogFileEnabled:                         logFileEnabled,
		LogDir:                                 filepath.Join(workDir, workDirLogsName),
		LogRetentionDays:                       logRetentionDays,
		AuthEnabled:                            authEnabled,
		LoginPassword:                          strings.TrimSpace(os.Getenv("LOGIN_PASSWORD")),
		AuthSessionTTL:                         authSessionTTL,
		AccountGuardEnabled:                    accountGuardEnabled,
		AccountGuardInterval:                   accountGuardInterval,
		AccountGuardUsageThreshold:             accountGuardUsageThreshold,
		AccountGuardWeeklyTokenLimit:           accountGuardWeeklyTokenLimit,
		AccountGuardProviderQuotaEnabled:       accountGuardProviderQuotaEnabled,
		AccountGuardDryRun:                     accountGuardDryRun,
		AccountGuardAutoReenable:               accountGuardAutoReenable,
		AccountGuardResetWeekday:               accountGuardResetWeekday,
		AccountGuardResetHour:                  accountGuardResetHour,
		AccountGuardRemoveBannedEnabled:        accountGuardRemoveBannedEnabled,
		AccountGuardRemoveBannedDryRun:         accountGuardRemoveBannedDryRun,
		AccountGuardRemoveBannedStatusMessages: accountGuardRemoveBannedStatusMessages,
	}
	if cfg.CPABaseURL == "" {
		return nil, fmt.Errorf("CPA_BASE_URL is required")
	}
	if cfg.CPAManagementKey == "" {
		return nil, fmt.Errorf("CPA_MANAGEMENT_KEY is required")
	}
	if cfg.AuthEnabled && cfg.LoginPassword == "" {
		return nil, fmt.Errorf("LOGIN_PASSWORD is required when AUTH_ENABLED is true")
	}
	cfg.resolveRelativePaths(envBaseDir)

	return cfg, nil
}

func applyProjectTimeZone() error {
	zoneName := strings.TrimSpace(os.Getenv("TZ"))
	if zoneName == "" {
		zoneName = DefaultTimeZone
		if err := os.Setenv("TZ", zoneName); err != nil {
			return fmt.Errorf("set default TZ: %w", err)
		}
	}
	location, err := time.LoadLocation(zoneName)
	if err != nil {
		return fmt.Errorf("TZ is invalid: %w", err)
	}
	time.Local = location
	return nil
}

func loadDotEnv(options LoadOptions) (string, error) {
	if strings.TrimSpace(options.EnvFile) != "" {
		return loadDotEnvFile(options.EnvFile, true)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	if loaded, err := loadOptionalDotEnv(filepath.Join(cwd, ".env")); err != nil || loaded {
		if loaded {
			return cwd, err
		}
		return "", err
	}

	exeDir, err := executableDir()
	if err != nil {
		return "", fmt.Errorf("get executable directory: %w", err)
	}
	loaded, err := loadOptionalDotEnv(filepath.Join(exeDir, ".env"))
	if loaded {
		return exeDir, err
	}
	return "", err
}

func loadOptionalDotEnv(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat .env: %w", err)
	}
	if err := godotenv.Overload(path); err != nil {
		return false, fmt.Errorf("load .env: %w", err)
	}
	return true, nil
}

func loadDotEnvFile(path string, required bool) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return "", nil
		}
		return "", fmt.Errorf("stat env file: %w", err)
	}
	if err := godotenv.Overload(path); err != nil {
		return "", fmt.Errorf("load env file: %w", err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve env file path: %w", err)
	}
	return filepath.Dir(absolutePath), nil
}

func (cfg *Config) resolveRelativePaths(baseDir string) {
	if baseDir == "" {
		return
	}
	cfg.WorkDir = resolveRelativePath(baseDir, cfg.WorkDir)
	cfg.SQLitePath = resolveRelativePath(baseDir, cfg.SQLitePath)
	cfg.LogDir = resolveRelativePath(baseDir, cfg.LogDir)
	cfg.BackupDir = resolveRelativePath(baseDir, cfg.BackupDir)
}

func resolveRelativePath(baseDir, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(baseDir, value)
}

func normalizeBasePath(value string) (string, error) {
	if value == "" || value == "/" {
		return "", nil
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("must start with '/'")
	}

	normalized := path.Clean(value)
	if normalized == "." || normalized == "/" {
		return "", nil
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	return normalized, nil
}

func getString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return duration, nil
}

func getBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a valid bool: %w", key, err)
	}
	return parsed, nil
}

func getInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	return parsed, nil
}

func getInt64(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	return parsed, nil
}

func getFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid number: %w", key, err)
	}
	return parsed, nil
}

func getStringList(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized := strings.ToLower(trimmed)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func parseWeekday(value string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sunday", "sun", "0":
		return time.Sunday, nil
	case "monday", "mon", "1":
		return time.Monday, nil
	case "tuesday", "tue", "2":
		return time.Tuesday, nil
	case "wednesday", "wed", "3":
		return time.Wednesday, nil
	case "thursday", "thu", "4":
		return time.Thursday, nil
	case "friday", "fri", "5":
		return time.Friday, nil
	case "saturday", "sat", "6":
		return time.Saturday, nil
	default:
		return time.Sunday, fmt.Errorf("must be a weekday name or 0-6")
	}
}
