#!/bin/bash
if [ -f .env ]; then
	source .env
fi
set -x
VERSION="${VERSION:=SNAPSHOT}"
CURDIR=${PWD}
PKGDIR=.ci/pkg

mkdir -p ${PKGDIR}

rm -rf ${PKGDIR}/cakeagent "./.ci/CakeAgent-$VERSION.pkg"

lipo -create .build/x86_64-apple-macosx/release/cakeagent .build/arm64-apple-macosx/release/cakeagent -output ${PKGDIR}/cakeagent

codesign --sign "Developer ID Application: Frederic BOLTZ (${TEAM_ID})" \
	--entitlements darwin/Resources/release.entitlements \
	--options runtime  --timestamp ${PKGDIR}/cakeagent

pkgbuild --root .ci/pkg/ \
		--identifier com.aldunelabs.cakeagent \
		--version $VERSION \
		--scripts .ci/pkg/scripts \
		--install-location "/usr/local/bin" \
		--sign "Developer ID Installer: Frederic BOLTZ (${TEAM_ID})" \
		"./.ci/CakeAgent-$VERSION.pkg"

if [[ "$VERSION" != SNAPSHOT* ]]; then
	xcrun notarytool submit "./.ci/CakeAgent-$VERSION.pkg" \
			--apple-id ${APPLE_ID} \
			--team-id ${TEAM_ID} \
			--password "${APP_PASSWORD}" \
			--wait

	xcrun stapler staple "./.ci/CakeAgent-$VERSION.pkg"
fi