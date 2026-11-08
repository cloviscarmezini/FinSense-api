package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                       string
	Environment                string
	SwaggerEnabled             bool
	DatabaseURL                string
	DBMaxOpenConns             int
	DBMaxIdleConns             int
	DBConnMaxLifetime          time.Duration
	JWTSecret                  string
	JWTAccessExpirationMinutes int
	JWTRefreshExpirationDays   int
	S3Endpoint                 string
	S3PublicURL                string
	S3Bucket                   string
	S3AccessKey                string
	S3SecretKey                string
	S3Region                   string
	S3UseSSL                   bool
	AIEngineURL                string
	AIManagerURL               string
	AdminPort                  string
	AdminAPIKey                string
	ModelsDir                  string
	HuggingFaceDefaultToken    string
}

func Load() *Config {
	return &Config{
		Port:                       getEnv("PORT", "8080"),
		Environment:                getEnv("ENVIRONMENT", "local"),
		SwaggerEnabled:             getEnvAsBool("SWAGGER_ENABLED", true),
		DatabaseURL:                getEnv("DATABASE_URL", "postgres://finsense:finsense_secret@localhost:5432/finsense_db?sslmode=disable"),
		DBMaxOpenConns:             getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:             getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime:          getEnvAsDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		JWTSecret:                  getEnv("JWT_SECRET", "finsense_super_secret_jwt_key_local_development_2026_xyz"),
		JWTAccessExpirationMinutes: getEnvAsInt("JWT_ACCESS_EXPIRATION_MINUTES", 15),
		JWTRefreshExpirationDays:   getEnvAsInt("JWT_REFRESH_EXPIRATION_DAYS", 30),
		S3Endpoint:                 getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3PublicURL:                getEnv("S3_PUBLIC_URL", "http://localhost:9000"),
		S3Bucket:                   getEnv("S3_BUCKET", "finsense-assets"),
		S3AccessKey:                getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:                getEnv("S3_SECRET_KEY", "minioadmin"),
		S3Region:                   getEnv("S3_REGION", "us-east-1"),
		S3UseSSL:                   getEnvAsBool("S3_USE_SSL", false),
		AIEngineURL:                getEnv("AI_ENGINE_URL", "http://localhost:8001"),
		AIManagerURL:               getEnv("AI_MANAGER_URL", "http://localhost:8082"),
		AdminPort:                  getEnv("ADMIN_PORT", "8082"),
		AdminAPIKey:                getEnv("ADMIN_API_KEY", "finsense_admin_secret_key_2026"),
		ModelsDir:                  getEnv("MODELS_DIR", "./models"),
		HuggingFaceDefaultToken:    getEnv("HUGGING_FACE_DEFAULT_TOKEN", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	valStr := getEnv(key, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsDuration(key string, defaultVal time.Duration) time.Duration {
	valStr := getEnv(key, "")
	if val, err := time.ParseDuration(valStr); err == nil {
		return val
	}
	return defaultVal
}
