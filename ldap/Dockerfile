# OpenLDAP 2.6 for the bundled demo stack.
#
# Built from the distribution package rather than a third-party wrapper image:
# osixia/openldap has been unmaintained since 2021 and is stuck on 2.4.57, and
# the bitnami namespace was withdrawn from Docker Hub in August 2025. Debian's
# slapd tracks upstream and keeps getting security updates, which is the whole
# point of moving off 2.4.
#
# 2.5 is the floor because that is where the ppolicy overlay gained pwdEndTime,
# the only attribute a directory actually enforces as an account expiry. See
# "Account expiry and locking" in the README.
FROM debian:13-slim

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        slapd \
        ldap-utils \
    && rm -rf /var/lib/apt/lists/*

# Demo values, applied while the directory is built (see build-directory) and
# matching the credentials the compose file and the README hand out. Build args
# rather than ENV so they do not linger in the running container's environment;
# override them if you fork this for anything other than a demo — but then set a
# real password.
ARG LDAP_DOMAIN="example.org"
ARG LDAP_ORGANISATION="Example Inc."
ARG LDAP_ADMIN_PASSWORD="admin"

COPY schema/sudo.ldif /bootstrap/sudo.ldif
COPY config/ppolicy-overlay.ldif /bootstrap/ppolicy-overlay.ldif
COPY config/accesslog.ldif /bootstrap/accesslog.ldif
COPY seed.ldif /bootstrap/seed.ldif
COPY build-directory.sh /usr/local/sbin/build-directory
COPY docker-entrypoint.sh /usr/local/sbin/docker-entrypoint

# The directory is populated here, not on first boot: a build failure is then a
# failed `docker compose build` with the slapd error in front of you, instead of
# a container that comes up half-seeded and fails much later in the app.
RUN /usr/local/sbin/build-directory

EXPOSE 389

# Log level 256 is "connections, operations and results" — enough to see who
# bound and what was refused, which is what you want from a stack you are
# pointing an LDAP admin UI at.
ENTRYPOINT ["/usr/local/sbin/docker-entrypoint"]
CMD ["/usr/sbin/slapd", "-h", "ldap:/// ldapi:///", "-u", "openldap", "-g", "openldap", "-d", "256"]
