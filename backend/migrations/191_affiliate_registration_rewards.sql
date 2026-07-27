-- Registration rewards reuse the affiliate ledger. Each inviter/invitee pair
-- may receive each side of the registration reward at most once.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ual_registration_inviter_reward_uniq
    ON user_affiliate_ledger (user_id, source_user_id)
    WHERE action = 'registration_inviter_reward';

CREATE UNIQUE INDEX IF NOT EXISTS idx_ual_registration_invitee_reward_uniq
    ON user_affiliate_ledger (user_id, source_user_id)
    WHERE action = 'registration_invitee_reward';

COMMENT ON COLUMN user_affiliate_ledger.action IS
    'accrue|transfer|registration_inviter_reward|registration_invitee_reward';
