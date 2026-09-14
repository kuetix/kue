package filejsondb

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/kuetix/helpers"
	"github.com/kuetix/uuid"
	jsondb "github.com/pnkj-kmr/simple-json-db"
)

type Collection struct {
	db         *DB
	collection jsondb.Collection
	name       string
	cache      *CollectionCache
	mu         sync.RWMutex
}

func NewCollection(db *DB, cache *CollectionCache, name string) *Collection {
	col, err := db.GetDB().Collection(name)
	if err != nil {
		panic(err)
	}
	cache.Warm(col.GetAllByName())

	return &Collection{
		db:         db,
		collection: col,
		name:       name,
		cache:      cache,
	}
}

// Name - returns the name of the collection
func (c *Collection) Name() string {
	return c.name
}

// DB - returns the underlying database
func (c *Collection) DB() jsondb.DB {
	return c.db.GetDB()
}

// Collection - returns the underlying collection
func (c *Collection) Collection() jsondb.Collection {
	return c.collection
}

// String - returns the string representation of the collection
func (c *Collection) String() string {
	return fmt.Sprintf("Collection: %s", c.name)
}

// Len - returns the number of records in the collection
func (c *Collection) Len() uint64 {
	return c.collection.Len()
}

// Clear - (Locked/Blocking) clears the cache
func (c *Collection) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.Purge()
}

// Cache - returns the cache for the collection
func (c *Collection) Cache() *CollectionCache {
	return c.cache
}

// SetCache - sets the cache for the collection
func (c *Collection) SetCache(cache *CollectionCache) {
	if cache == nil {
		return
	}
	c.cache = cache
}

// hashIndexes - hashes the value for the given indexes
func (c *Collection) hashIndexes(id string, value interface{}, foundIndexes *map[string]interface{}, indexes ...string) {
	maps, err := helpers.ToMapRecursive(value)
	if err != nil {
		return
	}
	for _, index := range indexes {
		if v, ok := maps[index]; ok {
			var raw string
			switch v.(type) {
			case string:
				raw = v.(string)
			case []byte:
				raw = string(v.([]byte))
			case uint, uint8, uint16, uint32, uint64, int, int8, int16, int32, int64, float32, float64:
				raw = fmt.Sprintf("%v", v)
			case complex64, complex128:
				raw = fmt.Sprintf("%v", v)
			case uintptr:
				raw = fmt.Sprintf("%v", v)
			case bool:
				raw = fmt.Sprintf("%v", v)
			case nil:
				raw = fmt.Sprintf("%v", v)
			default:
				continue
			}
			raw = uuid.Id(raw)
			if foundIndexes != nil && len(*foundIndexes) > 0 {
				if _, ok := (*foundIndexes)[raw]; ok {
					continue
				}
			}
			(*foundIndexes)[raw] = map[string]interface{}{
				"id":    id,
				"value": v,
				"hash":  raw,
				"index": index,
				"type":  fmt.Sprintf("%T", v),
			}
		}
	}
}

// LoadIndexes - (Non blocking / not locked) returns the index for the given id
func (c *Collection) LoadIndexes(id string) (idx string, indexes map[string]interface{}, err error) {
	idx = fmt.Sprintf("%s.idx", id)
	var rawIndex []byte
	rawIndex, err = c.collection.Get(idx)
	if err != nil {
		id = uuid.Id(id)
		idx = fmt.Sprintf("%s.idx", id)
		rawIndex, err = c.collection.Get(idx)
		if err != nil {
			return idx, nil, err
		}
	}

	err = json.Unmarshal(rawIndex, &indexes)
	if err != nil {
		return idx, nil, err
	}

	return idx, indexes, nil
}

// IsExistsIndexes - (Non blocking / not locked) checks if the index exists for the given id
func (c *Collection) IsExistsIndexes(id string) (idx string, exists bool, err error) {
	idx = fmt.Sprintf("%s.idx", id)
	_, err = c.collection.Get(idx)
	if err != nil {
		exists = false
		return idx, exists, err
	}
	exists = true
	return idx, exists, nil
}

// DeleteIndexes - (Non blocking / not locked) deletes the index for the given id
func (c *Collection) DeleteIndexes(id string) (idx string, err error) {
	idx = fmt.Sprintf("%s.idx", id)
	err = c.collection.Delete(idx)
	if err != nil {
		return idx, err
	}

	c.cache.Delete(idx)

	return idx, nil
}

// DoSet - (Non blocking / not locked) sets the value for the given id
func (c *Collection) DoSet(id string, value interface{}) error {
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if err := c.collection.Create(id, v); err != nil {
		return err
	}

	c.cache.Set(id, v)
	return nil
}

// GetIndexes - (Locked/Blocking) returns the index for the given id
func (c *Collection) GetIndexes(id string) (idx string, indexes map[string]interface{}, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.LoadIndexes(id)
}

// SetIndex - (Locked/Blocking) sets the index for the given id and value, creating or merging with existing indexes as needed
func (c *Collection) SetIndex(id string, value interface{}, indexes ...string) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.AddIndex(id, value, indexes...)
}

// AddIndex - (Non blocking / not locked) adds the index for the given id and value, creating or merging with existing indexes as needed
func (c *Collection) AddIndex(id string, value interface{}, indexes ...string) (err error) {
	idx, existsIndexes, err := c.LoadIndexes(id)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existsIndexes = make(map[string]interface{})
	}
	if existsIndexes == nil {
		existsIndexes = make(map[string]interface{})
	}
	idhash := uuid.Id(id)
	existsIndexes[idhash] = map[string]interface{}{
		"id":    id,
		"value": id,
		"hash":  idhash,
		"index": "id",
		"type":  "string",
	}
	c.hashIndexes(id, value, &existsIndexes, indexes...)

	for hash, v := range existsIndexes {
		if v == nil {
			continue
		}
		idx = fmt.Sprintf("%s.idx", hash)
		err = c.DoSet(idx, map[string]interface{}{
			hash: v,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// Set - (Locked/Blocking) sets the value for the given id and indexes
func (c *Collection) Set(id string, value interface{}, indexes ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if err = c.collection.Create(id, v); err != nil {
		return err
	}

	c.cache.Set(id, v)

	err = c.AddIndex(id, value, indexes...)
	if err != nil {
		return err
	}

	return nil
}

// Get - (Locked/Blocking) gets the value for the given id, checking the cache first and falling back
// to the underlying collection if not found, also checking indexes if the id is not found directly
func (c *Collection) Get(id string, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cached, ok := c.cache.Get(id); ok {
		return json.Unmarshal(cached, value)
	}

	hash := id
	v, errs := c.collection.Get(hash)
	if errs != nil {
		_, indexes, err := c.LoadIndexes(hash)
		if err != nil {
			hash = uuid.Id(id)
			_, indexes, err = c.LoadIndexes(hash)
			if err != nil {
				return errs
			}
		}
		i, ok := indexes[hash].(map[string]interface{})
		if !ok {
			i, ok = indexes[id].(map[string]interface{})
		}
		if ok {
			lid := i["id"].(string)
			var cached []byte
			if cached, ok = c.cache.Get(lid); ok {
				return json.Unmarshal(cached, value)
			}
			v, errs = c.collection.Get(lid)
			if errs != nil {
				return errs
			}
		} else {
			return errs
		}
	}
	c.cache.Set(id, v)

	return json.Unmarshal(v, value)
}

// Delete - (Locked/Blocking) deletes the item with the given id
func (c *Collection) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.collection.Delete(id); err != nil {
		return err
	}

	c.cache.Delete(id)

	if _, ok, _ := c.IsExistsIndexes(id); ok {
		idxName := fmt.Sprintf("%s.idx", id)
		if err := c.collection.Delete(idxName); err != nil {
			return err
		}
		c.cache.Delete(idxName)
	}

	return nil
}

// Exists - (Non blocking / not locked) checks if the item with the given id exists
func (c *Collection) Exists(id string) bool {
	if _, ok := c.cache.Get(id); ok {
		return true
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache.Get(id); ok {
		return true
	}

	_, err := c.collection.Get(id)
	return err == nil
}

// GetAll - (Locked/Blocking) returns all items in the collection
func (c *Collection) GetAll() map[string][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cached, ok := c.cache.GetAllIfComplete(); ok {
		return cached
	}

	data := c.collection.GetAllByName()
	c.cache.Warm(data)
	return data
}

// Update - (Locked/Blocking) updates the value for the given id, keeping existing indexes intact
func (c *Collection) Update(id string, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if err = c.collection.Create(id, v); err != nil {
		return err
	}

	c.cache.Set(id, v)

	err = c.AddIndex(id, value)
	if err != nil {
		return err
	}

	return nil
}
