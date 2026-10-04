DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'files_size_non_negative'
          AND conrelid = 'files'::regclass
    ) THEN
        ALTER TABLE files
        ADD CONSTRAINT files_size_non_negative
        CHECK (size >= 0);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'blobs_size_non_negative'
          AND conrelid = 'blobs'::regclass
    ) THEN
        ALTER TABLE blobs
        ADD CONSTRAINT blobs_size_non_negative
        CHECK (size >= 0);
    END IF;
END $$;
