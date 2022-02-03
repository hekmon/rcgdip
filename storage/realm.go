package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"git.mills.io/prologic/bitcask"
)

type RealmController struct {
	prefix []byte
	db     *bitcask.Bitcask
}

func (c *Controller) NewScoppedAccess(realm string) *RealmController {
	return &RealmController{
		prefix: []byte(realm + "_"),
		db:     c.db,
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
	return sb.db.Delete(sb.fqdnKey(key))
}

func (sb *RealmController) Get(key string, unmarshallAsJSON interface{}) (found bool, err error) {
	// Get raw value
	rawValue, err := sb.db.Get(sb.fqdnKey(key))
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
	return sb.db.Has(sb.fqdnKey(key))

}

func (sb *RealmController) Keys() (keys []string) {
	for key := range sb.db.Keys() {
		if sb.hasKeyPrefix(key) {
			keys = append(keys, string(key[len(sb.prefix):]))
		}
	}
	return
}

func (sb *RealmController) NbKeys() (nbKeys int) {
	for key := range sb.db.Keys() {
		if sb.hasKeyPrefix(key) {
			nbKeys++
		}
	}
	return
}

func (sb *RealmController) Set(key string, marshall2JSON interface{}) (err error) {
	// Unmarshall raw value
	rawValue, err := json.Marshal(marshall2JSON)
	if err != nil {
		return
	}
	// Set raw value
	if err = sb.db.Put(sb.fqdnKey(key), rawValue); err != nil {
		return
	}
	// All good
	return
}

func (sb *RealmController) Sync() (err error) {
	return sb.db.Sync()
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
