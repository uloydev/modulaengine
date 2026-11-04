package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrMissingDataURL    = errors.New("missing data pool database URL")
	ErrMissingControlURL = errors.New("missing control pool database URL")
	ErrPoolInit          = errors.New("failed to initialize connection pool")
)

type Config struct {
	DataURL         string
	ControlURL      string
	ReadURL         string
	MaxConnsData    int32
	MaxConnsControl int32
	MinConns        int32
	MaxConnIdleTime time.Duration
	MaxConnLifetime time.Duration
}

func DefaultConfig(dataURL, controlURL string) Config {
	return Config{
		DataURL:         dataURL,
		ControlURL:      controlURL,
		MaxConnsData:    25,
		MaxConnsControl: 5,
		MinConns:        2,
		MaxConnIdleTime: 15 * time.Minute,
		MaxConnLifetime: 1 * time.Hour,
	}
}

type PoolManager struct {
	DataPool    *pgxpool.Pool
	ControlPool *pgxpool.Pool
	ReadPool    *pgxpool.Pool
}

func NewPoolManager(ctx context.Context, cfg Config) (*PoolManager, error) {
	if cfg.DataURL == "" {
		return nil, ErrMissingDataURL
	}
	if cfg.ControlURL == "" {
		return nil, ErrMissingControlURL
	}

	dataConfig, err := pgxpool.ParseConfig(cfg.DataURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse data url: %w", ErrPoolInit, err)
	}
	dataConfig.MaxConns = cfg.MaxConnsData
	dataConfig.MinConns = cfg.MinConns
	dataConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	dataConfig.MaxConnLifetime = cfg.MaxConnLifetime

	dataPool, err := pgxpool.NewWithConfig(ctx, dataConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: create data pool: %w", ErrPoolInit, err)
	}

	controlConfig, err := pgxpool.ParseConfig(cfg.ControlURL)
	if err != nil {
		dataPool.Close()
		return nil, fmt.Errorf("%w: parse control url: %w", ErrPoolInit, err)
	}
	controlConfig.MaxConns = cfg.MaxConnsControl
	controlConfig.MinConns = 1
	controlConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	controlConfig.MaxConnLifetime = cfg.MaxConnLifetime

	controlPool, err := pgxpool.NewWithConfig(ctx, controlConfig)
	if err != nil {
		dataPool.Close()
		return nil, fmt.Errorf("%w: create control pool: %w", ErrPoolInit, err)
	}

	var readPool *pgxpool.Pool
	if cfg.ReadURL != "" {
		readConfig, err := pgxpool.ParseConfig(cfg.ReadURL)
		if err != nil {
			dataPool.Close()
			controlPool.Close()
			return nil, fmt.Errorf("%w: parse read url: %w", ErrPoolInit, err)
		}
		readConfig.MaxConns = cfg.MaxConnsData
		readConfig.MinConns = cfg.MinConns
		readConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
		readConfig.MaxConnLifetime = cfg.MaxConnLifetime

		readPool, err = pgxpool.NewWithConfig(ctx, readConfig)
		if err != nil {
			dataPool.Close()
			controlPool.Close()
			return nil, fmt.Errorf("%w: create read pool: %w", ErrPoolInit, err)
		}
	}

	return &PoolManager{
		DataPool:    dataPool,
		ControlPool: controlPool,
		ReadPool:    readPool,
	}, nil
}

func (m *PoolManager) Reader() *pgxpool.Pool {
	if m.ReadPool != nil {
		return m.ReadPool
	}
	return m.DataPool
}

func (m *PoolManager) Writer() *pgxpool.Pool {
	return m.DataPool
}

func (m *PoolManager) Controller() *pgxpool.Pool {
	return m.ControlPool
}

func (m *PoolManager) Ping(ctx context.Context) error {
	if m.DataPool != nil {
		if err := m.DataPool.Ping(ctx); err != nil {
			return fmt.Errorf("ping data pool: %w", err)
		}
	}
	if m.ControlPool != nil {
		if err := m.ControlPool.Ping(ctx); err != nil {
			return fmt.Errorf("ping control pool: %w", err)
		}
	}
	if m.ReadPool != nil {
		if err := m.ReadPool.Ping(ctx); err != nil {
			return fmt.Errorf("ping read pool: %w", err)
		}
	}
	return nil
}

func (m *PoolManager) Close() {
	if m.DataPool != nil {
		m.DataPool.Close()
	}
	if m.ControlPool != nil {
		m.ControlPool.Close()
	}
	if m.ReadPool != nil {
		m.ReadPool.Close()
	}
}
