# Data model

`collection_targets` stores the current YAML target definition, next due time, and lease. `collection_runs` records one execution with status and counts. `observations` stores source fields and the original parsed card JSONB. Prices are signed 64-bit integer minor currency units; missing price is NULL. Source URLs and IDs remain text. `observation_heads` tracks the last saved content hash and time for each target/product/offer. `collector_state` stores one process heartbeat and circuit state.

The dedupe hash uses title, URL, prices, currency, seller, availability, image, and selected generic source fields such as delivery, badge, and rating. Rank is omitted from the hash to avoid churn from sorting. Every changed hash saves a new observation. An unchanged hash saves again after 24 hours. This policy is target-specific and does not delete history. Raw JSON is retained for saved observations, not every repeated identical card in every response.

Migrations live in `internal/storage/migrations` and are embedded in the executable. They define the tables and analytics views from a clean schema. There is no migration path from any other project database.
