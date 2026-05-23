-- IP 历史突增请求检测脚本
--
-- 用法：
-- 1. 默认扫描 usage_logs 全量历史。
-- 2. 需要缩小范围时，修改 params 里的 start_at / end_at。
-- 3. SQLite 的时间字符串请按 UTC 存储格式填写，例如 '2026-05-22 00:00:00'。
-- 4. PostgreSQL 可填写带时区时间，例如 TIMESTAMPTZ '2026-05-22 00:00:00+08'。
--
-- 数据来源：
-- usage_logs(client_ip, created_at, status_code, total_tokens, user_billed,
--            endpoint, inbound_endpoint, model, effective_model, api_key_id, api_key_name)
--
-- 阈值建议：
-- min_req_per_second = 5       同一秒 5 次以上请求
-- min_req_per_10s    = 10      10 秒内 10 次以上请求
-- spike_ratio        = 3       当前桶 >= 该 IP 前序历史均值 3 倍
--
-- PostgreSQL 查询

-- 1. 历史同一秒大量请求
WITH params AS (
  SELECT
    NULL::timestamptz AS start_at,
    NULL::timestamptz AS end_at,
    5::int AS min_req_per_second
),
per_second AS (
  SELECT
    TRIM(client_ip) AS ip,
    date_trunc('second', created_at) AS bucket_at,
    COUNT(*) AS requests,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys,
    STRING_AGG(DISTINCT COALESCE(NULLIF(effective_model, ''), NULLIF(model, ''), '-'), ', ') AS models
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), date_trunc('second', created_at)
)
SELECT
  ip,
  bucket_at,
  requests,
  tokens,
  ROUND(cost::numeric, 6) AS cost,
  errors,
  api_keys,
  models
FROM per_second, params
WHERE requests >= params.min_req_per_second
ORDER BY requests DESC, bucket_at DESC
LIMIT 200;

-- 2. 历史 10 秒桶突增检测：当前 10 秒请求量与同 IP 前 30 个非空 10 秒桶比较
WITH params AS (
  SELECT
    NULL::timestamptz AS start_at,
    NULL::timestamptz AS end_at,
    10::int AS min_req_per_10s,
    3.0::float AS spike_ratio,
    5::int AS min_baseline_buckets
),
bucket_10s AS (
  SELECT
    TRIM(client_ip) AS ip,
    to_timestamp(FLOOR(EXTRACT(EPOCH FROM created_at) / 10) * 10) AS bucket_at,
    COUNT(*) AS requests,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), to_timestamp(FLOOR(EXTRACT(EPOCH FROM created_at) / 10) * 10)
),
with_baseline AS (
  SELECT
    *,
    AVG(requests) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS prev_avg_requests,
    MAX(requests) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS prev_max_requests,
    COUNT(*) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS baseline_buckets
  FROM bucket_10s
)
SELECT
  ip,
  bucket_at,
  requests,
  ROUND(COALESCE(prev_avg_requests, 0)::numeric, 2) AS prev_avg_requests,
  COALESCE(prev_max_requests, 0) AS prev_max_requests,
  ROUND((requests / GREATEST(COALESCE(prev_avg_requests, 0.1), 0.1))::numeric, 2) AS spike_ratio,
  tokens,
  ROUND(cost::numeric, 6) AS cost,
  errors,
  api_keys
FROM with_baseline, params
WHERE requests >= params.min_req_per_10s
  AND baseline_buckets >= params.min_baseline_buckets
  AND requests >= GREATEST(
    params.min_req_per_10s,
    COALESCE(prev_avg_requests, 0) * params.spike_ratio,
    COALESCE(prev_max_requests, 0) + 5
  )
ORDER BY spike_ratio DESC, requests DESC, bucket_at DESC
LIMIT 200;

-- 3. 历史单 IP 高频时间段排行：按分钟聚合，适合快速定位异常时间段
WITH params AS (
  SELECT
    NULL::timestamptz AS start_at,
    NULL::timestamptz AS end_at,
    10::int AS min_req_per_minute
),
per_minute AS (
  SELECT
    TRIM(client_ip) AS ip,
    date_trunc('minute', created_at) AS bucket_at,
    COUNT(*) AS requests,
    COUNT(*) AS rpm,
    SUM(COALESCE(total_tokens, 0)) AS tpm,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    MIN(created_at) AS first_seen_at,
    MAX(created_at) AS last_seen_at
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), date_trunc('minute', created_at)
)
SELECT
  ip,
  bucket_at,
  requests,
  rpm,
  tpm,
  tokens,
  ROUND(cost::numeric, 6) AS cost,
  errors,
  first_seen_at,
  last_seen_at
FROM per_minute, params
WHERE requests >= params.min_req_per_minute
ORDER BY rpm DESC, tpm DESC, bucket_at DESC
LIMIT 200;

-- 4. 历史 IP 总体排行：查出历史高频 IP，便于再按 IP 做追踪
WITH params AS (
  SELECT
    NULL::timestamptz AS start_at,
    NULL::timestamptz AS end_at
)
SELECT
  TRIM(client_ip) AS ip,
  COUNT(*) AS requests,
  SUM(COALESCE(total_tokens, 0)) AS tokens,
  ROUND(SUM(COALESCE(user_billed, 0))::numeric, 6) AS cost,
  SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
  MIN(created_at) AS first_seen_at,
  MAX(created_at) AS last_seen_at,
  COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys
FROM usage_logs, params
WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
  AND (params.start_at IS NULL OR created_at >= params.start_at)
  AND (params.end_at IS NULL OR created_at < params.end_at)
GROUP BY TRIM(client_ip)
ORDER BY requests DESC, tokens DESC
LIMIT 200;

-- SQLite 查询

-- 1. 历史同一秒大量请求
WITH params AS (
  SELECT
    NULL AS start_at,
    NULL AS end_at,
    5 AS min_req_per_second
),
per_second AS (
  SELECT
    TRIM(client_ip) AS ip,
    strftime('%Y-%m-%d %H:%M:%S', created_at) AS bucket_at,
    COUNT(*) AS requests,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys,
    GROUP_CONCAT(DISTINCT COALESCE(NULLIF(effective_model, ''), NULLIF(model, ''), '-')) AS models
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), strftime('%Y-%m-%d %H:%M:%S', created_at)
)
SELECT
  ip,
  bucket_at,
  requests,
  tokens,
  ROUND(cost, 6) AS cost,
  errors,
  api_keys,
  models
FROM per_second, params
WHERE requests >= params.min_req_per_second
ORDER BY requests DESC, bucket_at DESC
LIMIT 200;

-- 2. 历史 10 秒桶突增检测：当前 10 秒请求量与同 IP 前 30 个非空 10 秒桶比较
WITH params AS (
  SELECT
    NULL AS start_at,
    NULL AS end_at,
    10 AS min_req_per_10s,
    3.0 AS spike_ratio,
    5 AS min_baseline_buckets
),
bucket_10s AS (
  SELECT
    TRIM(client_ip) AS ip,
    datetime((CAST(strftime('%s', created_at) AS INTEGER) / 10) * 10, 'unixepoch') AS bucket_at,
    COUNT(*) AS requests,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), datetime((CAST(strftime('%s', created_at) AS INTEGER) / 10) * 10, 'unixepoch')
),
with_baseline AS (
  SELECT
    *,
    AVG(requests) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS prev_avg_requests,
    MAX(requests) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS prev_max_requests,
    COUNT(*) OVER (
      PARTITION BY ip
      ORDER BY bucket_at
      ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING
    ) AS baseline_buckets
  FROM bucket_10s
)
SELECT
  ip,
  bucket_at,
  requests,
  ROUND(COALESCE(prev_avg_requests, 0), 2) AS prev_avg_requests,
  COALESCE(prev_max_requests, 0) AS prev_max_requests,
  ROUND(requests / MAX(COALESCE(prev_avg_requests, 0.1), 0.1), 2) AS spike_ratio,
  tokens,
  ROUND(cost, 6) AS cost,
  errors,
  api_keys
FROM with_baseline, params
WHERE requests >= params.min_req_per_10s
  AND baseline_buckets >= params.min_baseline_buckets
  AND requests >= MAX(
    params.min_req_per_10s,
    COALESCE(prev_avg_requests, 0) * params.spike_ratio,
    COALESCE(prev_max_requests, 0) + 5
  )
ORDER BY spike_ratio DESC, requests DESC, bucket_at DESC
LIMIT 200;

-- 3. 历史单 IP 高频时间段排行：按分钟聚合，适合快速定位异常时间段
WITH params AS (
  SELECT
    NULL AS start_at,
    NULL AS end_at,
    10 AS min_req_per_minute
),
per_minute AS (
  SELECT
    TRIM(client_ip) AS ip,
    strftime('%Y-%m-%d %H:%M:00', created_at) AS bucket_at,
    COUNT(*) AS requests,
    COUNT(*) AS rpm,
    SUM(COALESCE(total_tokens, 0)) AS tpm,
    SUM(COALESCE(total_tokens, 0)) AS tokens,
    SUM(COALESCE(user_billed, 0)) AS cost,
    SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
    MIN(created_at) AS first_seen_at,
    MAX(created_at) AS last_seen_at
  FROM usage_logs, params
  WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
    AND (params.start_at IS NULL OR created_at >= params.start_at)
    AND (params.end_at IS NULL OR created_at < params.end_at)
  GROUP BY TRIM(client_ip), strftime('%Y-%m-%d %H:%M:00', created_at)
)
SELECT
  ip,
  bucket_at,
  requests,
  rpm,
  tpm,
  tokens,
  ROUND(cost, 6) AS cost,
  errors,
  first_seen_at,
  last_seen_at
FROM per_minute, params
WHERE requests >= params.min_req_per_minute
ORDER BY rpm DESC, tpm DESC, bucket_at DESC
LIMIT 200;

-- 4. 历史 IP 总体排行：查出历史高频 IP，便于再按 IP 做追踪
WITH params AS (
  SELECT
    NULL AS start_at,
    NULL AS end_at
)
SELECT
  TRIM(client_ip) AS ip,
  COUNT(*) AS requests,
  SUM(COALESCE(total_tokens, 0)) AS tokens,
  ROUND(SUM(COALESCE(user_billed, 0)), 6) AS cost,
  SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS errors,
  MIN(created_at) AS first_seen_at,
  MAX(created_at) AS last_seen_at,
  COUNT(DISTINCT NULLIF(api_key_id, 0)) AS api_keys
FROM usage_logs, params
WHERE NULLIF(TRIM(client_ip), '') IS NOT NULL
  AND (params.start_at IS NULL OR created_at >= params.start_at)
  AND (params.end_at IS NULL OR created_at < params.end_at)
GROUP BY TRIM(client_ip)
ORDER BY requests DESC, tokens DESC
LIMIT 200;
