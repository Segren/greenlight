package main

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Segren/greenlight/internal/data"
	"github.com/Segren/greenlight/internal/jsonlog"
	"github.com/Segren/greenlight/internal/mailer"

	_ "github.com/lib/pq"
)

var (
	buildTime string
	version   string
)

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	//для конфигурации кол-ва запросов в секунду и во время бурста
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	//для отправки почты по протоколу smtp
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	//cross origin запросы
	cors struct {
		trustedOrigins []string
	}
}

var (
	instance *config
	once     sync.Once
)

// singleton
func GetConfig() *config {
	once.Do(func() {
		instance = &config{}
		flag.IntVar(&instance.port, "port", 8080, "API server port")
		flag.StringVar(&instance.env, "env", "development", "Environment (development|staging|production)")

		flag.StringVar(&instance.db.dsn, "db-dsn", "", "PostgreSQL DSN")

		flag.IntVar(&instance.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
		flag.IntVar(&instance.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
		flag.StringVar(&instance.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")

		flag.Float64Var(&instance.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
		flag.IntVar(&instance.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
		flag.BoolVar(&instance.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

		//по умолчанию - параметры ящика в Mailtrap
		flag.StringVar(&instance.smtp.host, "smtp-host", "smtp.mailtrap.io", "SMTP host")
		flag.IntVar(&instance.smtp.port, "smtp-port", 25, "SMTP port")
		flag.StringVar(&instance.smtp.username, "smtp-username", "02426e5654fbb8", "SMTP username")
		flag.StringVar(&instance.smtp.password, "smtp-password", "9d29d9263dabf0", "SMTP password")
		flag.StringVar(&instance.smtp.sender, "smtp-sender", "Greenlight <noreply@greenlight.example.com>", "SMTP sender email address")

		flag.Func("cors-trusted-origins", "Trusted CORS origins (space separated)", func(val string) error {
			instance.cors.trustedOrigins = strings.Fields(val)
			return nil
		})

		flag.Parse()
	})
	return instance
}

type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
	mailer mailer.Mailer
	wg     sync.WaitGroup
}

func main() {
	cfg := GetConfig()

	// булево для отображения версии проекта и выхода
	displayVersion := flag.Bool("version", false, "Display version information and exit")

	if *displayVersion {
		fmt.Printf("Greenlight version:\t%s\n", version)
		fmt.Printf("Build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	if cfg.db.dsn == "" {
		cfg.db.dsn = os.Getenv("GREENLIGHT_DB_DSN")
		if cfg.db.dsn == "" {
			logger.PrintFatal(errors.New("должна быть установлена строка подключения к базе данных через флаг -db-dsn или переменную окружения GREENLIGHT_DB_DSN"), nil)
		}
	}

	logger.PrintInfo("Connecting to database with DSN: "+cfg.db.dsn, nil)

	db, err := openDB(*cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	defer db.Close()

	logger.PrintInfo("database connection pool established", nil)

	//метрика версии
	expvar.NewString("version").Set(version)

	//метрика кол-ва активных горутин
	expvar.Publish("goroutines", expvar.Func(func() interface{} {
		return runtime.NumGoroutine()
	}))

	//метрика по пулу подключений БД
	expvar.Publish("database", expvar.Func(func() interface{} {
		return db.Stats()
	}))

	//текущее время
	expvar.Publish("timestamp", expvar.Func(func() interface{} {
		return time.Now().Unix()
	}))

	app := &application{
		config: *cfg,
		logger: logger,
		models: data.NewModels(db),
		mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
	}

	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, nil)
	}
}

// возвращает пул подключений дб
func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}
