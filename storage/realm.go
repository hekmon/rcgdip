package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"git.mills.io/prologic/bitcask"
)

type RealmController struct {
	name   string
	prefix []byte
	main   *Controller
}

func (c *Controller) NewScoppedAccess(realm string) *RealmController {
	return &RealmController{
		name:   realm,
		prefix: []byte(realm + "_"),
		main:   c,
	}
}

/*
	public methods
*/

func (sb *RealmController) Clear() (err error) {
	for _, key := range sb.Keys() {
		if err = sb.Delete(key); err != nil {
			return fmt.Errorf("failed to delete key '%s': %w", key, err)
		}
	}
	return
}

func (sb *RealmController) Delete(key string) (err error) {
	return sb.main.db.Delete(sb.fqdnKey(key))
}

func (sb *RealmController) Get(key string, unmarshallAsJSON interface{}) (found bool, err error) {
	// Get raw value
	rawValue, err := sb.main.db.Get(sb.fqdnKey(key))
	if err != nil {
		if errors.Is(err, bitcask.ErrKeyNotFound) {
			err = nil
		}
		return
	}
	found = true
	// Unmarshall raw value
	if err = json.Unmarshal(rawValue, unmarshallAsJSON); err != nil {
		return
	}
	// All good
	return
}

func (sb *RealmController) Has(key string) (exists bool) {
	return sb.main.db.Has(sb.fqdnKey(key))

}

func (sb *RealmController) Keys() (keys []string) {
	for key := range sb.main.db.Keys() {
		if sb.hasKeyPrefix(key) {
			keys = append(keys, string(key[len(sb.prefix):]))
		}
	}
	return
}

func (sb *RealmController) NbKeys() (nbKeys int) {
	for key := range sb.main.db.Keys() {
		if sb.hasKeyPrefix(key) {
			nbKeys++
		}
	}
	return
}

func (sb *RealmController) Set(key string, marshall2JSON interface{}) (err error) {
	// Marshall raw value
	rawValue, err := json.Marshal(marshall2JSON)
	if err != nil {
		return
	}
	// Set raw value
	rawKey := sb.fqdnKey(key)
	if err = sb.main.db.Put(rawKey, rawValue); err != nil {
		return
	}
	// All good, update stats
	sb.main.updateKeysStat(len(rawKey))
	sb.main.updateValuesStat(len(rawValue))
	return
}

func (sb *RealmController) Sync() (err error) {
	return sb.main.db.Sync()
}

/*
	private methods
*/

func (sb *RealmController) fqdnKey(key string) (fullKey []byte) {
	fullKey = make([]byte, len(sb.prefix), len(sb.prefix)+len(key))
	copy(fullKey, sb.prefix)
	return append(fullKey, []byte(key)...)
}

func (sb *RealmController) hasKeyPrefix(rawKey []byte) bool {
	if len(rawKey) < len(sb.prefix) {
		return false
	}
	rawKeyPrefix := rawKey[:(len(sb.prefix))]
	return reflect.DeepEqual(rawKeyPrefix, sb.prefix)
}
