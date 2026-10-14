#!/bin/sh
# Build the demo directory at image build time: configure the database, load the
# schema and overlay the app needs, then add the seed entries. The result is
# stashed as a skeleton that docker-entrypoint copies into empty volumes.
set -eu

SKEL_CONFIG=/opt/ldapwarden/skel/slapd.d
SKEL_DATA=/opt/ldapwarden/skel/ldap

# --- database -----------------------------------------------------------------
#
# purge_database is safe and wanted: the package's postinst has just created a
# dc=nodomain directory that nothing has written to yet.
debconf-set-selections <<EOF
slapd slapd/no_configuration boolean false
slapd slapd/domain string ${LDAP_DOMAIN}
slapd shared/organization string ${LDAP_ORGANISATION}
slapd slapd/password1 password ${LDAP_ADMIN_PASSWORD}
slapd slapd/password2 password ${LDAP_ADMIN_PASSWORD}
slapd slapd/backend select MDB
slapd slapd/purge_database boolean true
slapd slapd/move_old_database boolean true
slapd slapd/allow_ldap_v2 boolean false
EOF
dpkg-reconfigure -f noninteractive slapd

BASE_DN=$(printf '%s' "${LDAP_DOMAIN}" | sed 's/^/dc=/; s/\./,dc=/g')
ADMIN_DN="cn=admin,${BASE_DN}"

# --- a slapd to talk to -------------------------------------------------------
#
# Over ldapi:// as root, which maps to the cn=config rootdn via SASL EXTERNAL —
# no config password needed, and nothing listens on a TCP port during the build.
mkdir -p /run/slapd
chown openldap:openldap /run/slapd
/usr/sbin/slapd -h "ldapi:///" -u openldap -g openldap

for _ in $(seq 1 30); do
    ldapsearch -Y EXTERNAL -H ldapi:/// -b "cn=config" -s base dn >/dev/null 2>&1 && break
    sleep 0.2
done
ldapsearch -Y EXTERNAL -H ldapi:/// -b "cn=config" -s base dn >/dev/null

# Shut down via the pid file rather than pkill: debian:*-slim has no procps, and
# the pid file is where slapd itself says it is.
stop_slapd() {
    [ -f /run/slapd/slapd.pid ] || return 0
    pid=$(cat /run/slapd/slapd.pid)
    kill "${pid}" 2>/dev/null || return 0
    for _ in $(seq 1 50); do
        kill -0 "${pid}" 2>/dev/null || return 0
        sleep 0.2
    done
    echo "slapd did not shut down" >&2
    return 1
}
trap 'stop_slapd || true' EXIT

# --- schema -------------------------------------------------------------------
#
# sudo only. ppolicy's schema is *built into the overlay* from 2.5 onwards, so
# loading a ppolicy.ldif here would fail on duplicate attribute types — that
# file is a 2.4-ism and is deliberately not shipped any more.
ldapadd -Y EXTERNAL -H ldapi:/// -f /bootstrap/sudo.ldif

# --- ppolicy overlay ----------------------------------------------------------
#
# The overlay LDIF adds olcModuleLoad to cn=module{0}, which only exists once
# something has loaded a module. Debian links back_mdb dynamically so it is
# normally there; create it if this ever changes, so the file below stays the
# single portable description of the overlay (the README points operators at it
# for their own directories).
if ! ldapsearch -Y EXTERNAL -H ldapi:/// -b "cn=module{0},cn=config" -s base dn >/dev/null 2>&1; then
    ldapadd -Y EXTERNAL -H ldapi:/// <<'EOF'
dn: cn=module,cn=config
objectClass: olcModuleList
cn: module
olcModulePath: /usr/lib/ldap
EOF
fi
ldapmodify -Y EXTERNAL -H ldapi:/// -f /bootstrap/ppolicy-overlay.ldif

# --- seed ---------------------------------------------------------------------
#
# Added online, as the rootdn: the overlay hashes the cleartext userPassword
# values on the way in (olcPPolicyHashCleartext), and binding as the rootdn
# skips the quality checks that would otherwise reject a seed password.
ldapadd -x -H ldapi:/// -D "${ADMIN_DN}" -w "${LDAP_ADMIN_PASSWORD}" -f /bootstrap/seed.ldif

# --- accesslog ----------------------------------------------------------------
#
# A second database recording every write, for the app's view of changes made
# outside it. After the seed, so the seed is not logged as a burst of "external
# changes" dated from the image build. The database directory has to exist,
# owned by slapd's user, before the database is added; it lives under
# /var/lib/ldap, so the skeleton below — and therefore the volume — carries it.
mkdir -p /var/lib/ldap/accesslog
chown openldap:openldap /var/lib/ldap/accesslog
ldapmodify -Y EXTERNAL -H ldapi:/// -f /bootstrap/accesslog.ldif

# Fail the build rather than ship a directory that cannot do the one thing this
# image exists for.
ldapsearch -Y EXTERNAL -H ldapi:/// -b "cn=Subschema" -s base attributeTypes \
    | grep -q "'pwdEndTime'" \
    || { echo "pwdEndTime missing from the subschema: ppolicy overlay not active" >&2; exit 1; }
# The overlay's own configuration, not cn=accesslog: that entry only appears
# with the first write logged after the overlay is enabled.
ldapsearch -Y EXTERNAL -H ldapi:/// -b "cn=config" "(olcOverlay=accesslog)" dn \
    | grep -q "^dn: olcOverlay" \
    || { echo "accesslog overlay not configured" >&2; exit 1; }

stop_slapd
trap - EXIT

# --- skeleton -----------------------------------------------------------------
mkdir -p "${SKEL_CONFIG}" "${SKEL_DATA}"
cp -a /etc/ldap/slapd.d/. "${SKEL_CONFIG}/"
cp -a /var/lib/ldap/. "${SKEL_DATA}/"
