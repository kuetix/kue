package filejsondb

import jsondb "github.com/pnkj-kmr/simple-json-db"

type DB struct {
	Db              jsondb.DB
	CacheMaxBytes   int
	CacheMaxObjects int
}

func NewDB(dbPath string, cacheSettings ...int) (*DB, error) {
	if len(cacheSettings) == 0 {
		cacheSettings = []int{defaultCacheMaxBytes, defaultCacheMaxObjects}
	}
	if len(cacheSettings) == 1 {
		cacheSettings = append(cacheSettings, defaultCacheMaxObjects)
	}
	cacheMaxBytes, cacheMaxObjects := cacheSettings[0], cacheSettings[1]

	db, err := jsondb.New(dbPath, &jsondb.Options{UseGzip: false})
	if err != nil {
		return nil, err
	}

	return &DB{
		Db:              db,
		CacheMaxBytes:   cacheMaxBytes,
		CacheMaxObjects: cacheMaxObjects,
	}, nil
}

func (d *DB) GetDB() jsondb.DB {
	return d.Db
}

func (d *DB) NewCollection(name string) *Collection {
	cache := NewCollectionCache(d.CacheMaxObjects, d.CacheMaxBytes)
	return NewCollection(d, cache, name)
}
