CREATE TYPE otp_status AS ENUM ('sent', 'failed');

CREATE TABLE otp_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id),
    device_id UUID NOT NULL REFERENCES devices (id),
    phone VARCHAR(20) NOT NULL,
    wamid VARCHAR(128),
    status otp_status NOT NULL DEFAULT 'sent',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_logs_team_id ON otp_logs (team_id);
CREATE INDEX idx_otp_logs_phone ON otp_logs (team_id, phone);
