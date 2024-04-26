WITH agg as (
    SELECT code_hash, group_concat(package_path) AS paths, COUNT(1) AS count FROM realms GROUP BY code_hash ORDER BY count DESC
) SELECT * FROM agg WHERE count > 1;