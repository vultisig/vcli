-- Seed plugins for local development
-- Run with: make seed

INSERT INTO plugins (id, title, description, server_endpoint, category, logo_url, thumbnail_url, images, features, faqs, audited, created_at, updated_at)
VALUES
(
    'vultisig-fees-feee',
    'Vultisig Fees',
    'Automatic fee collection for Vultisig transactions',
    'http://vultisig-fee:8085',
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
    'http://vultisig-dca:8082',
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
    'http://vultisig-dca:8083',
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
