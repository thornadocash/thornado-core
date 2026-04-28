use redb::{Database, TableDefinition};
use serde::{de::DeserializeOwned, Serialize};
use std::path::Path;
use std::sync::Arc;

const KV_TABLE: TableDefinition<&str, &[u8]> = TableDefinition::new("kv");

pub type Result<T> = std::result::Result<T, Error>;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("store error: {0}")]
    Store(String),
    #[error("json error: {0}")]
    Json(String),
}

pub trait KvStore {
    fn get(&self, key: &str) -> Result<Option<Vec<u8>>>;
    fn put(&self, key: &str, value: &[u8]) -> Result<()>;
    fn delete(&self, key: &str) -> Result<()>;
}

#[derive(Clone)]
pub struct RedbKvStore {
    db: Arc<Database>,
}

impl RedbKvStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self> {
        let path = path.as_ref();
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).map_err(store_error)?;
        }
        let db = Database::create(path)
            .or_else(|_| Database::open(path))
            .map_err(store_error)?;
        {
            let write = db.begin_write().map_err(store_error)?;
            write.open_table(KV_TABLE).map_err(store_error)?;
            write.commit().map_err(store_error)?;
        }
        Ok(Self { db: Arc::new(db) })
    }
}

impl KvStore for RedbKvStore {
    fn get(&self, key: &str) -> Result<Option<Vec<u8>>> {
        let read = self.db.begin_read().map_err(store_error)?;
        let table = read.open_table(KV_TABLE).map_err(store_error)?;
        table
            .get(key)
            .map(|value| value.map(|bytes| bytes.value().to_vec()))
            .map_err(store_error)
    }

    fn put(&self, key: &str, value: &[u8]) -> Result<()> {
        let write = self.db.begin_write().map_err(store_error)?;
        {
            let mut table = write.open_table(KV_TABLE).map_err(store_error)?;
            table.insert(key, value).map_err(store_error)?;
        }
        write.commit().map_err(store_error)
    }

    fn delete(&self, key: &str) -> Result<()> {
        let write = self.db.begin_write().map_err(store_error)?;
        {
            let mut table = write.open_table(KV_TABLE).map_err(store_error)?;
            table.remove(key).map_err(store_error)?;
        }
        write.commit().map_err(store_error)
    }
}

pub fn put_json<T: Serialize>(store: &impl KvStore, key: &str, value: &T) -> Result<()> {
    let bytes = serde_json::to_vec(value).map_err(json_error)?;
    store.put(key, &bytes)
}

pub fn get_json<T: DeserializeOwned>(store: &impl KvStore, key: &str) -> Result<Option<T>> {
    store
        .get(key)?
        .map(|bytes| serde_json::from_slice(&bytes).map_err(json_error))
        .transpose()
}

fn store_error<E: std::fmt::Display>(error: E) -> Error {
    Error::Store(error.to_string())
}

fn json_error(error: serde_json::Error) -> Error {
    Error::Json(error.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn redb_store_roundtrips_json() {
        let path = std::env::temp_dir().join(format!(
            "thornado-store-test-{}-{}.redb",
            std::process::id(),
            "roundtrip"
        ));
        let _ = std::fs::remove_file(&path);
        let store = RedbKvStore::open(&path).unwrap();
        put_json(&store, "thing", &vec!["a", "b"]).unwrap();
        let value: Vec<String> = get_json(&store, "thing").unwrap().unwrap();
        assert_eq!(value, ["a", "b"]);
        store.delete("thing").unwrap();
        assert!(get_json::<Vec<String>>(&store, "thing").unwrap().is_none());
        drop(store);
        let _ = std::fs::remove_file(path);
    }
}
