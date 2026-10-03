# Analytics views

- `latest_observations`: newest row for each target/product/offer.
- `price_history`: time series of observed price, reference price, and seller.
- `product_price_summary`: minimum, maximum, and latest observed price per identity.
- `target_daily_stats`: saved observations and unique identities by target/day.
- `seller_stats`: saved observations and unique identities by seller/target.
- `collection_run_stats`: totals and latest successful run by target.
- `collector_health`: heartbeat, queue, running count, completed count, latest success.

All values represent what the source displayed, not verified market prices. The materialized history is deliberately simple. Use `observations.raw_metadata` to build new analyses without changing collection behavior.
