#!/bin/sh
# Seed empty volumes from the skeleton built into the image, then hand over to
# slapd.
#
# "Empty" is the only trigger: a directory that already has data is left exactly
# as it is, so restarting the stack never silently reverts the changes you made
# through the app. To go back to the seed, remove the volumes.
set -eu

SKEL_CONFIG=/opt/ldapwarden/skel/slapd.d
SKEL_DATA=/opt/ldapwarden/skel/ldap

restore_if_empty() {
    skel=$1
    target=$2
    mkdir -p "${target}"
    if [ -z "$(ls -A "${target}" 2>/dev/null)" ]; then
        echo "ldapwarden: seeding ${target} from the bundled directory"
        cp -a "${skel}/." "${target}/"
    fi
    chown -R openldap:openldap "${target}"
}

restore_if_empty "${SKEL_CONFIG}" /etc/ldap/slapd.d
restore_if_empty "${SKEL_DATA}" /var/lib/ldap

mkdir -p /run/slapd
chown openldap:openldap /run/slapd

exec "$@"
