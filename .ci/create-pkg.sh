#!/bin/bash
if [ -f .env ]; then
	source .env
fi

VERSION=${VERSION:=SNAPSHOT}
CURDIR=${PWD}
PKGDIR=.ci/pkg

mkdir -p ${PKGDIR}

lipo -create .build/x86_64-apple-macosx/release/cakeagent .build/arm64-apple-macosx/release/cakeagent -output ${PKGDIR}/cakeagent

if [ -n "$1" ]; then
	KEYCHAIN_OPTIONS="--keychain $1"
else
	KEYCHAIN_OPTIONS=
fi

pkgbuild --root .ci/pkg/ \
		--identifier com.aldunelabs.cakeagent \
		--version $VERSION \
		--scripts .ci/pkg/scripts \
		--install-location "/usr/local/bin" \
		--sign "Developer ID Installer: Frederic BOLTZ (${TEAM_ID})" \
		${KEYCHAIN_OPTIONS} \
		"./.ci/CakeAgent-$VERSION.pkg"

xcrun notarytool submit "./.ci/CakeAgent-$VERSION.pkg" ${KEYCHAIN_OPTIONS} \
		--apple-id ${APPLE_ID} \
		--team-id ${TEAM_ID} \
		--password "${APP_PASSWORD}" \
		--wait

xcrun stapler staple "./.ci/CakeAgent-$VERSION.pkg"
