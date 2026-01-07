-- Create databases for all services
CREATE DATABASE "vultisig-verifier";
CREATE DATABASE "vultisig-dca";
CREATE DATABASE "vultisig-fee";

-- Grant all privileges to vultisig user
GRANT ALL PRIVILEGES ON DATABASE "vultisig-verifier" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-dca" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-fee" TO vultisig;
