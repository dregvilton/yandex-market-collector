CREATE OR REPLACE VIEW latest_observations AS
SELECT DISTINCT ON (target_id, source_product_id, source_offer_id) o.*
FROM observations o
ORDER BY target_id, source_product_id, source_offer_id, observed_at DESC, id DESC;
CREATE OR REPLACE VIEW price_history AS
SELECT t.key AS target_key,o.source_product_id,o.source_offer_id,o.observed_at,o.price_minor,o.reference_price_minor,o.currency,o.seller_name
FROM observations o JOIN collection_targets t ON t.id=o.target_id;
CREATE OR REPLACE VIEW product_price_summary AS
SELECT t.key AS target_key,o.source_product_id,o.source_offer_id,min(o.price_minor) AS min_price_minor,max(o.price_minor) AS max_price_minor,
 (array_agg(o.price_minor ORDER BY o.observed_at DESC,o.id DESC))[1] AS latest_price_minor,
 count(*) AS observations
FROM observations o JOIN collection_targets t ON t.id=o.target_id
GROUP BY t.key,o.source_product_id,o.source_offer_id;
CREATE OR REPLACE VIEW target_daily_stats AS
SELECT t.key AS target_key,date_trunc('day',o.observed_at) AS day,count(*) AS observations,
 count(DISTINCT (o.source_product_id,o.source_offer_id)) AS unique_products
FROM observations o JOIN collection_targets t ON t.id=o.target_id GROUP BY t.key,date_trunc('day',o.observed_at);
CREATE OR REPLACE VIEW seller_stats AS
SELECT t.key AS target_key,o.seller_source_id,o.seller_name,count(*) AS observations,
 count(DISTINCT (o.source_product_id,o.source_offer_id)) AS unique_products
FROM observations o JOIN collection_targets t ON t.id=o.target_id
WHERE o.seller_source_id<>'' OR o.seller_name<>'' GROUP BY t.key,o.seller_source_id,o.seller_name;
CREATE OR REPLACE VIEW collection_run_stats AS
SELECT t.key AS target_key,count(r.id) AS runs,count(*) FILTER (WHERE r.status='success') AS successful_runs,
 max(r.completed_at) FILTER (WHERE r.status='success') AS latest_success,
 sum(r.raw_items) AS raw_items,sum(r.parsed_items) AS parsed_items,sum(r.saved_items) AS saved_items,
 sum(r.challenges) AS challenges,sum(r.http_403) AS http_403,sum(r.http_429) AS http_429
FROM collection_targets t LEFT JOIN collection_runs r ON r.target_id=t.id GROUP BY t.key;
CREATE OR REPLACE VIEW collector_health AS
SELECT s.*, (now()-s.heartbeat_at < interval '30 seconds') AS heartbeat_fresh,
 (SELECT count(*) FROM collection_targets WHERE enabled AND due_at<=now()) AS queued_targets,
 (SELECT count(*) FROM collection_runs WHERE status='running') AS running_collections,
 (SELECT count(*) FROM collection_runs WHERE status='success') AS completed_collections,
 (SELECT max(completed_at) FROM collection_runs WHERE status='success') AS latest_success
FROM collector_state s;
