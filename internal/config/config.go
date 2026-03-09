package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	Redis       RedisConfig
	JWT         JWTConfig
	OTP         OTPConfig
	Twilio      TwilioConfig
	Paystack    PaystackConfig
	Flutterwave FlutterwaveConfig
	Worker      WorkerConfig
	GoogleMaps  GoogleMapsConfig
	S3          S3Config
	Firebase    FirebaseConfig
}

type FirebaseConfig struct {
	CredentialsPath string
}

type WorkerConfig struct {
	Enabled                  bool
	EscrowCheckInterval      time.Duration
	RiderSearchCheckInterval time.Duration
	LinkExpiryCheckInterval  time.Duration
	RadiusExpandInterval     time.Duration
	TimerExpiryCheckInterval time.Duration
	RiderSearchTimeoutMins   int
	RadiusExpandSteps        []int
	RadiusExpandIntervalMins int
	RiderOfferTimeoutMins    int
	MaxJobRestarts   int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	JitterPercent        int
	ReminderCooldownMins int
}

type ServerConfig struct {
	Port              string
	Environment       string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	AdminSignupSecret string
}

type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host         string
	Port         int
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
}

type JWTConfig struct {
	AccessSecret       string
	RefreshSecret      string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	Issuer             string
}

type OTPConfig struct {
	Length          int
	ExpiryDuration  time.Duration
	MaxAttempts     int
	RateLimitCount  int
	RateLimitWindow time.Duration
	ResendCooldown  time.Duration
	BcryptCost      int
}

type TwilioConfig struct {
	AccountSID       string
	AuthToken        string
	VerifyServiceSID string
}

type PaystackConfig struct {
	SecretKey string
	PublicKey string
	BaseURL   string
}

type FlutterwaveConfig struct {
	SecretKey string
	PublicKey string
	BaseURL   string
}

type GoogleMapsConfig struct {
	APIKey string
}

type S3Config struct {
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	BaseURL         string
	MaxFileSizeMB   int
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:              viper.GetString("server.port"),
			Environment:       viper.GetString("server.environment"),
			ReadTimeout:       viper.GetDuration("server.read_timeout"),
			WriteTimeout:      viper.GetDuration("server.write_timeout"),
			AdminSignupSecret: viper.GetString("server.admin_signup_secret"),
		},
		Database: DatabaseConfig{
			Host:            viper.GetString("database.host"),
			Port:            viper.GetInt("database.port"),
			User:            viper.GetString("database.user"),
			Password:        viper.GetString("database.password"),
			DBName:          viper.GetString("database.dbname"),
			SSLMode:         viper.GetString("database.sslmode"),
			MaxOpenConns:    viper.GetInt("database.max_open_conns"),
			MaxIdleConns:    viper.GetInt("database.max_idle_conns"),
			ConnMaxLifetime: viper.GetDuration("database.conn_max_lifetime"),
		},
		Redis: RedisConfig{
			Host:         viper.GetString("redis.host"),
			Port:         viper.GetInt("redis.port"),
			Password:     viper.GetString("redis.password"),
			DB:           viper.GetInt("redis.db"),
			PoolSize:     viper.GetInt("redis.pool_size"),
			MinIdleConns: viper.GetInt("redis.min_idle_conns"),
		},
		JWT: JWTConfig{
			AccessSecret:       viper.GetString("jwt.access_secret"),
			RefreshSecret:      viper.GetString("jwt.refresh_secret"),
			AccessTokenExpiry:  viper.GetDuration("jwt.access_token_expiry"),
			RefreshTokenExpiry: viper.GetDuration("jwt.refresh_token_expiry"),
			Issuer:             viper.GetString("jwt.issuer"),
		},
		OTP: OTPConfig{
			Length:          viper.GetInt("otp.length"),
			ExpiryDuration:  viper.GetDuration("otp.expiry_duration"),
			MaxAttempts:     viper.GetInt("otp.max_attempts"),
			RateLimitCount:  viper.GetInt("otp.rate_limit_count"),
			RateLimitWindow: viper.GetDuration("otp.rate_limit_window"),
			ResendCooldown:  viper.GetDuration("otp.resend_cooldown"),
			BcryptCost:      viper.GetInt("otp.bcrypt_cost"),
		},
		Twilio: TwilioConfig{
			AccountSID:       viper.GetString("twilio.account_sid"),
			AuthToken:        viper.GetString("twilio.auth_token"),
			VerifyServiceSID: viper.GetString("twilio.verify_service_sid"),
		},
		Paystack: PaystackConfig{
			SecretKey: viper.GetString("paystack.secret_key"),
			PublicKey: viper.GetString("paystack.public_key"),
			BaseURL:   viper.GetString("paystack.base_url"),
		},
		Flutterwave: FlutterwaveConfig{
			SecretKey: viper.GetString("flutterwave.secret_key"),
			PublicKey: viper.GetString("flutterwave.public_key"),
			BaseURL:   viper.GetString("flutterwave.base_url"),
		},
		Worker: WorkerConfig{
			Enabled:                  viper.GetBool("worker.enabled"),
			EscrowCheckInterval:      viper.GetDuration("worker.escrow_check_interval"),
			RiderSearchCheckInterval: viper.GetDuration("worker.rider_search_check_interval"),
			LinkExpiryCheckInterval:  viper.GetDuration("worker.link_expiry_check_interval"),
			RadiusExpandInterval:     viper.GetDuration("worker.radius_expand_interval"),
			TimerExpiryCheckInterval: viper.GetDuration("worker.timer_expiry_check_interval"),
			RiderSearchTimeoutMins:   viper.GetInt("worker.rider_search_timeout_mins"),
			RadiusExpandSteps:        viper.GetIntSlice("worker.radius_expand_steps"),
			RadiusExpandIntervalMins: viper.GetInt("worker.radius_expand_interval_mins"),
			RiderOfferTimeoutMins:    viper.GetInt("worker.rider_offer_timeout_mins"),
			MaxJobRestarts:           viper.GetInt("worker.max_job_restarts"),
			InitialBackoff:           viper.GetDuration("worker.initial_backoff"),
			MaxBackoff:               viper.GetDuration("worker.max_backoff"),
			JitterPercent:            viper.GetInt("worker.jitter_percent"),
			ReminderCooldownMins:     viper.GetInt("worker.reminder_cooldown_mins"),
		},
		GoogleMaps: GoogleMapsConfig{
			APIKey: viper.GetString("google_maps.api_key"),
		},
		S3: S3Config{
			Region:          viper.GetString("aws.region"),
			Bucket:          viper.GetString("aws.s3_bucket"),
			AccessKeyID:     viper.GetString("aws.access_key_id"),
			SecretAccessKey: viper.GetString("aws.secret_access_key"),
			BaseURL:         viper.GetString("aws.s3_base_url"),
			MaxFileSizeMB:   viper.GetInt("aws.max_file_size_mb"),
		},
		Firebase: FirebaseConfig{
			CredentialsPath: viper.GetString("firebase.credentials_path"),
		},
	}

	return cfg, nil
}

func setDefaults() {
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.environment", "development")
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.admin_signup_secret", "")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "kipa")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("database.conn_max_lifetime", "5m")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.min_idle_conns", 2)
	viper.SetDefault("jwt.access_token_expiry", "168h") // 1 week for development
	viper.SetDefault("jwt.refresh_token_expiry", "168h") // 7 days
	viper.SetDefault("jwt.issuer", "kipa")
	viper.SetDefault("otp.length", 6)
	viper.SetDefault("otp.expiry_duration", "5m")
	viper.SetDefault("otp.max_attempts", 3)
	viper.SetDefault("otp.rate_limit_count", 5)
	viper.SetDefault("otp.rate_limit_window", "1h")
	viper.SetDefault("otp.resend_cooldown", "60s")
	viper.SetDefault("otp.bcrypt_cost", 10)

	viper.SetDefault("paystack.base_url", "https://api.paystack.co")
	viper.SetDefault("flutterwave.base_url", "https://api.flutterwave.com/v3")
	viper.SetDefault("worker.enabled", true)
	viper.SetDefault("worker.escrow_check_interval", "1m")
	viper.SetDefault("worker.rider_search_check_interval", "30s")
	viper.SetDefault("worker.link_expiry_check_interval", "5m")
	viper.SetDefault("worker.radius_expand_interval", "2m")
	viper.SetDefault("worker.timer_expiry_check_interval", "1m")
	viper.SetDefault("worker.rider_search_timeout_mins", 3)
	viper.SetDefault("worker.radius_expand_steps", []int{3, 5, 8, 12})
	viper.SetDefault("worker.radius_expand_interval_mins", 3)
	viper.SetDefault("worker.rider_offer_timeout_mins", 2)
	viper.SetDefault("worker.max_job_restarts", 10)
	viper.SetDefault("worker.initial_backoff", "1s")
	viper.SetDefault("worker.max_backoff", "5m")
	viper.SetDefault("worker.jitter_percent", 20)
	viper.SetDefault("worker.reminder_cooldown_mins", 60)
	viper.SetDefault("aws.region", "us-east-1")
	viper.SetDefault("aws.max_file_size_mb", 5)
	viper.SetDefault("firebase.credentials_path", "./firebase-service-account.json")
}

func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}
