-- Seed plugins for local development
-- Run with: make seed

INSERT INTO plugins (id, title, description, server_endpoint, category, logo_url, thumbnail_url, images, features, faqs, audited, created_at, updated_at)
VALUES
(
    'vultisig-fees-feee',
    'Vultisig Fees',
    'Automatic fee collection for Vultisig transactions',
    'http://localhost:8085',
    'plugin',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/fees/icon.jpg',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/fees/thumbnail.jpg',
    '[]',
    '["Automatic fee deduction", "Multi-chain support", "Transparent pricing"]',
    '[]',
    false,
    NOW(),
    NOW()
),
(
    'vultisig-dca-0000',
    'DCA (Dollar Cost Averaging)',
    'Automated recurring swaps and transfers',
    'http://localhost:8082',
    'app',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/dca/icon.jpg',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/dca/thumbnail.jpg',
    '[]',
    '["Recurring swaps", "Multi-chain support", "Flexible scheduling"]',
    '[]',
    false,
    NOW(),
    NOW()
),
(
    'vultisig-recurring-sends-0000',
    'Recurring Sends',
    'Automated recurring token transfers',
    'http://localhost:8083',
    'app',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/recurring-sends/icon.jpg',
    'https://raw.githubusercontent.com/vultisig/verifier/main/assets/plugins/recurring-sends/thumbnail.jpg',
    '[]',
    '["Scheduled transfers", "Multi-chain support", "Reliable execution"]',
    '[]',
    false,
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    server_endpoint = EXCLUDED.server_endpoint,
    category = EXCLUDED.category,
    updated_at = NOW();

-- Seed plugin API keys
INSERT INTO plugin_apikey (plugin_id, apikey, status)
VALUES
    ('vultisig-fees-feee', 'local-dev-fee-apikey', 1),
    ('vultisig-dca-0000', 'local-dev-dca-apikey', 1),
    ('vultisig-recurring-sends-0000', 'local-dev-send-apikey', 1)
ON CONFLICT (apikey) DO NOTHING;

-- Seed plugin pricing (required for policy creation)
-- Each plugin needs pricing entries that match the billing types used in policies
-- Types: 'once' (one-time fee), 'per-tx' (per transaction), 'recurring' (subscription)
-- For 'once' and 'per-tx', frequency must be NULL
-- For 'recurring', frequency must be: daily, weekly, biweekly, or monthly
-- Note: Delete existing rows first to prevent duplicates (pricings table has no unique constraint on type+plugin_id)
DELETE FROM pricings WHERE plugin_id IN ('vultisig-dca-0000', 'vultisig-recurring-sends-0000', 'vultisig-fees-feee');
INSERT INTO pricings (type, frequency, amount, asset, metric, plugin_id, created_at, updated_at)
VALUES
    ('once', NULL, 0, 'usdc', 'fixed', 'vultisig-dca-0000', NOW(), NOW()),
    ('per-tx', NULL, 0, 'usdc', 'fixed', 'vultisig-dca-0000', NOW(), NOW()),
    ('once', NULL, 0, 'usdc', 'fixed', 'vultisig-recurring-sends-0000', NOW(), NOW()),
    ('per-tx', NULL, 0, 'usdc', 'fixed', 'vultisig-recurring-sends-0000', NOW(), NOW()),
    ('once', NULL, 0, 'usdc', 'fixed', 'vultisig-fees-feee', NOW(), NOW()),
    ('per-tx', NULL, 0, 'usdc', 'fixed', 'vultisig-fees-feee', NOW(), NOW());
