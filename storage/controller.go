package storage

import (
	"context"
	"fmt"
	"sync"

	"git.mills.io/prologic/bitcask"
	"github.com/hekmon/hllogger"
)

type Config struct {
	Instance string
	Logger   *hllogger.HlLogger
}

type Controller struct {
	// Global
	logger       *hllogger.HlLogger
	mainDBPath   string
	backupDBPath string
	// KV DB
	db *bitcask.Bitcask
	// Workers
	ctx       context.Context
	ctxCancel func()
	workers   sync.WaitGroup
}

func New(conf Config) (c *Controller, err error) {
	// Base init
	if conf.Instance != "" {
		conf.Instance = "_" + conf.Instance
	}
	c = &Controller{
		logger:       conf.Logger,
		mainDBPath:   fmt.Sprintf("rcgdip_storage%s", conf.Instance),
		backupDBPath: fmt.Sprintf("rcgdip_storage%s_backup", conf.Instance),
	}
	// Open up the db
	if c.db, err = bitcask.Open(c.mainDBPath); err != nil {
		return
	}
	c.logger.Info("[Storage] db successfully open")
	// Create a backup
	if err = c.db.Backup(c.backupDBPath); err != nil {
		return
	}
	c.logger.Debug("[Storage] db backup successfull")
	// Start the warden
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.workers.Add(1)
	go c.warden()
	return
}

func (c *Controller) Stop() {
	c.logger.Debug("[Storage] stop signal received, stopping workers...")
	c.ctxCancel()
	c.workers.Wait()
	c.logger.Debug("[Storage] workers stopped, closing the db...")
	if err := c.db.Close(); err != nil {
		c.logger.Errorf("[Storage] can not cleanly close the db, it might get corrupt, please consider using the backup db before restarting: %s",
			err.Error())
	} else {
		c.logger.Info("[Storage] database closed")
	}
}
