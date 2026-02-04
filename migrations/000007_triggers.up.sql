-- Migration: 000007_triggers
-- Description: Create trigger functions and triggers for progress notifications and timestamp updates
-- Created: 2026-02-04

-- notify_review_progress function
-- Sends NOTIFY event when progress events are inserted for real-time UI updates
CREATE OR REPLACE FUNCTION notify_review_progress()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    payload JSONB;
BEGIN
    -- Build notification payload
    payload := jsonb_build_object(
        'event_id', NEW.id,
        'request_id', NEW.request_id,
        'event_type', NEW.event_type,
        'event_data', NEW.event_data,
        'created_at', NEW.created_at
    );

    -- Send notification on 'review_progress' channel
    PERFORM pg_notify('review_progress', payload::text);

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION notify_review_progress IS 'Sends NOTIFY event for real-time progress updates to connected clients';

-- Trigger for review progress notifications
CREATE TRIGGER trg_review_progress_notify
    AFTER INSERT ON review_progress_events
    FOR EACH ROW
    EXECUTE FUNCTION notify_review_progress();

COMMENT ON TRIGGER trg_review_progress_notify ON review_progress_events IS 'Triggers NOTIFY on new progress events for real-time UI updates';

-- update_papers_timestamp function
-- Automatically updates the updated_at timestamp on papers table modifications
CREATE OR REPLACE FUNCTION update_papers_timestamp()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION update_papers_timestamp IS 'Automatically sets updated_at timestamp on row update';

-- Trigger for papers updated_at
CREATE TRIGGER trg_papers_updated_at
    BEFORE UPDATE ON papers
    FOR EACH ROW
    EXECUTE FUNCTION update_papers_timestamp();

COMMENT ON TRIGGER trg_papers_updated_at ON papers IS 'Automatically updates papers.updated_at on modification';

-- update_review_request_timestamp function
-- Automatically updates the updated_at timestamp on literature_review_requests
CREATE OR REPLACE FUNCTION update_review_request_timestamp()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION update_review_request_timestamp IS 'Automatically sets updated_at timestamp on review request update';

-- Trigger for literature_review_requests updated_at
CREATE TRIGGER trg_review_requests_updated_at
    BEFORE UPDATE ON literature_review_requests
    FOR EACH ROW
    EXECUTE FUNCTION update_review_request_timestamp();

COMMENT ON TRIGGER trg_review_requests_updated_at ON literature_review_requests IS 'Automatically updates literature_review_requests.updated_at on modification';

-- update_request_paper_mapping_timestamp function
-- Automatically updates the updated_at timestamp on request_paper_mappings
CREATE OR REPLACE FUNCTION update_request_paper_mapping_timestamp()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION update_request_paper_mapping_timestamp IS 'Automatically sets updated_at timestamp on request paper mapping update';

-- Trigger for request_paper_mappings updated_at
CREATE TRIGGER trg_request_paper_mappings_updated_at
    BEFORE UPDATE ON request_paper_mappings
    FOR EACH ROW
    EXECUTE FUNCTION update_request_paper_mapping_timestamp();

COMMENT ON TRIGGER trg_request_paper_mappings_updated_at ON request_paper_mappings IS 'Automatically updates request_paper_mappings.updated_at on modification';

-- update_paper_sources_timestamp function
-- Automatically updates the updated_at timestamp on paper_sources
CREATE OR REPLACE FUNCTION update_paper_sources_timestamp()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION update_paper_sources_timestamp IS 'Automatically sets updated_at timestamp on paper source update';

-- Trigger for paper_sources updated_at
CREATE TRIGGER trg_paper_sources_updated_at
    BEFORE UPDATE ON paper_sources
    FOR EACH ROW
    EXECUTE FUNCTION update_paper_sources_timestamp();

COMMENT ON TRIGGER trg_paper_sources_updated_at ON paper_sources IS 'Automatically updates paper_sources.updated_at on modification';
