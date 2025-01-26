#!/bin/bash
set -ex
# create variables
RUNNER_TEMP=${RUNNER_TEMP:=$PWD}
CERTIFICATE_PATH=$RUNNER_TEMP/installer_certificate.p12

# import certificate and provisioning profile from secrets
echo -n "$BUILD_CERTIFICATE_BASE64" | base64 -d > $CERTIFICATE_PATH

# create temporary keychain
security create-keychain -p "$KEYCHAIN_PASSWORD" build.keychain
security default-keychain -s build.keychain
security set-keychain-settings -lut 21600 build.keychain
security unlock-keychain -p "$KEYCHAIN_PASSWORD" build.keychain

# import certificate to keychain
security import $CERTIFICATE_PATH -P "$P12_PASSWORD" -A -t cert -f pkcs12 -k build.keychain -T /usr/bin/codesign -T /usr/bin/pkgbuild
security set-key-partition-list -S apple-tool:,apple: -k "$KEYCHAIN_PASSWORD" build.keychain
security list-keychain -d user -s build.keychain
