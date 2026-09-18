# Bolt's Journal

## 2026-07-20 - Batching Database Operations for Multi-Field Updates
**Optimization:** Previously, `handlePutSettings` looped over the input map and issued separate SQLite `INSERT ... ON CONFLICT` operations for every individual key-value pair, each incurring transaction and disk I/O overhead.
**Learning:** Sequential single-row writes in SQLite incur substantial synchronous disk fsync overhead per transaction. Grouping related row operations within a single explicit transaction (`BeginTx`) using a prepared statement reduces I/O latency from O(N) fsyncs to O(1).
**Prevention / Pattern:** Whenever updating multiple key-value attributes or bulk database records, expose a batched storage method (e.g., `PutSettings`) that wraps operations in a single database transaction.
