UPDATE objects
SET created_at = CASE
  WHEN instr(created_at, '.') = 0 THEN substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
  ELSE substr(created_at, 1, instr(created_at, '.')) ||
       substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9) || 'Z'
END,
unreferenced_at = CASE
  WHEN unreferenced_at IS NULL THEN NULL
  WHEN instr(unreferenced_at, '.') = 0 THEN substr(unreferenced_at, 1, length(unreferenced_at) - 1) || '.000000000Z'
  ELSE substr(unreferenced_at, 1, instr(unreferenced_at, '.')) ||
       substr(substr(unreferenced_at, instr(unreferenced_at, '.') + 1, length(unreferenced_at) - instr(unreferenced_at, '.') - 1) || '000000000', 1, 9) || 'Z'
END;

UPDATE artifact_records
SET created_at = CASE
  WHEN instr(created_at, '.') = 0 THEN substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
  ELSE substr(created_at, 1, instr(created_at, '.')) ||
       substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9) || 'Z'
END,
deleted_at = CASE
  WHEN deleted_at IS NULL THEN NULL
  WHEN instr(deleted_at, '.') = 0 THEN substr(deleted_at, 1, length(deleted_at) - 1) || '.000000000Z'
  ELSE substr(deleted_at, 1, instr(deleted_at, '.')) ||
       substr(substr(deleted_at, instr(deleted_at, '.') + 1, length(deleted_at) - instr(deleted_at, '.') - 1) || '000000000', 1, 9) || 'Z'
END;
