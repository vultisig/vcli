-- Create databases for all services
CREATE DATABASE "vultisig-verifier";
CREATE DATABASE "vultisig-dca";
CREATE DATABASE "vultisig-sends";
CREATE DATABASE "vultisig-fee";
CREATE DATABASE "vultisig-agent";

-- Grant all privileges to vultisig user
GRANT ALL PRIVILEGES ON DATABASE "vultisig-verifier" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-dca" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-sends" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-fee" TO vultisig;
GRANT ALL PRIVILEGES ON DATABASE "vultisig-agent" TO vultisig;
