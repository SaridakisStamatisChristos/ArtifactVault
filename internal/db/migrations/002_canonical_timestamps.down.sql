UPDATE objects
SET created_at = CASE
  WHEN substr(created_at, 21, 9) = '000000000' THEN substr(created_at, 1, 19) || 'Z'
  ELSE substr(created_at, 1, 20) || rtrim(substr(created_at, 21, 9), '0') || 'Z'
END,
unreferenced_at = CASE
  WHEN unreferenced_at IS NULL THEN NULL
  WHEN substr(unreferenced_at, 21, 9) = '000000000' THEN substr(unreferenced_at, 1, 19) || 'Z'
  ELSE substr(unreferenced_at, 1, 20) || rtrim(substr(unreferenced_at, 21, 9), '0') || 'Z'
END;

UPDATE artifact_records
SET created_at = CASE
  WHEN substr(created_at, 21, 9) = '000000000' THEN substr(created_at, 1, 19) || 'Z'
  ELSE substr(created_at, 1, 20) || rtrim(substr(created_at, 21, 9), '0') || 'Z'
END,
deleted_at = CASE
  WHEN deleted_at IS NULL THEN NULL
  WHEN substr(deleted_at, 21, 9) = '000000000' THEN substr(deleted_at, 1, 19) || 'Z'
  ELSE substr(deleted_at, 1, 20) || rtrim(substr(deleted_at, 21, 9), '0') || 'Z'
END;
