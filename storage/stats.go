package storage

func (c *Controller) loadStats() {
	c.logger.Debug("[Storage] loading stats...")
	c.statsAccess.Lock()
	if found, err := c.statsRealm.Get(maxSizeKeyKey, &c.maxSizeKey); err != nil {
		c.logger.Errorf("[Storage] failed to load the %s stat value: %s", maxSizeKeyKey, err.Error())
	} else if found {
		c.logger.Debugf("[Storage] loaded stat %s: %d", maxSizeKeyKey, c.maxSizeKey)
	} else {
		c.logger.Debugf("[Storage] no saved stat %s found", maxSizeKeyKey)
	}
	if found, err := c.statsRealm.Get(maxSizeValueKey, &c.maxSizeValue); err != nil {
		c.logger.Errorf("[Storage] failed to save the %s stat value: %s", maxSizeValueKey, err.Error())
	} else if found {
		c.logger.Debugf("[Storage] loaded stat %s: %d", maxSizeValueKey, c.maxSizeValue)
	} else {
		c.logger.Debugf("[Storage] no saved stat %s found", maxSizeValueKey)
	}
	c.statsAccess.Unlock()
}

func (c *Controller) saveStats() {
	c.logger.Debug("[Storage] saving stats...")
	c.statsAccess.Lock()
	if err := c.statsRealm.Set(maxSizeKeyKey, c.maxSizeKey); err != nil {
		c.logger.Errorf("[Storage] failed to save the %s stats value: %s", maxSizeKeyKey, err.Error())
	}
	if err := c.statsRealm.Set(maxSizeValueKey, c.maxSizeValue); err != nil {
		c.logger.Errorf("[Storage] failed to save the %s stats value: %s", maxSizeValueKey, err.Error())
	}
	c.logger.Debugf("[Storage] saved stats: max size key encoutered is %d and max size value encoutered is %d", c.maxSizeKey, c.maxSizeValue)
	c.statsAccess.Unlock()
}

func (c *Controller) updateKeysStat(keyLength int) {
	c.workers.Add(1)
	go func() {
		c.statsAccess.Lock()
		if keyLength > c.maxSizeKey {
			c.maxSizeKey = keyLength
		}
		c.statsAccess.Unlock()
		c.workers.Done()
	}()
}

func (c *Controller) updateValuesStat(valueLength int) {
	c.workers.Add(1)
	go func() {
		c.statsAccess.Lock()
		if valueLength > c.maxSizeValue {
			c.maxSizeValue = valueLength
		}
		c.statsAccess.Unlock()
		c.workers.Done()
	}()
}
