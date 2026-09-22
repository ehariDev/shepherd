#!/usr/bin/env python3
"""Offline validator for Shepherd collector-label keys/values and matcher-safety.

Mirrors, byte-for-byte, the rules enforced server-side by:
  - internal/mgmtapi/rpc_fleet.go: validCollectorLabelKey / validCollectorLabelValue
  - internal/merge/reserved.go: IsReserved
  - the Alertmanager matcher grammar (github.com/prometheus/alertmanager/pkg/labels)
    used by internal/merge/merge.go's MatchesPipeline, for --matcher-safe

No network access, no dependency on a running Shepherd instance. Use this before
calling the live API so validation failures are caught locally.

Usage:
  validate_label.py <key> <value>        # full admin-label validation
  validate_label.py --matcher-safe <key> # check if key can be used in a pipeline matcher
"""
import sys
import unicodedata

MAX_KEY_LEN = 128
MAX_VALUE_LEN = 512
MAX_LABELS = 64  # informational only — this script validates one key/value at a time

RESERVED_EXACT = {"cluster", "role", "id", "os", "alloy_version"}
RESERVED_PREFIXES = ("collector.", "shepherd.")

# cluster/role are reserved from being SET as admin labels (SetCollectorLabel
# rejects them) but are the two keys BuildCollectorLabels always populates as
# built-ins (merge.go: labels["cluster"]=..., labels["role"]=...) -- they are
# the most matcher-safe keys that exist. id/os/alloy_version are reserved AND
# never added to the built label map by BuildCollectorLabels, so they are
# correctly unusable in a matcher today even though they're real collector
# fields shown elsewhere in the UI.
BUILTIN_MATCHER_KEYS = {"cluster", "role"}

LABEL_KEY_CHARS = set("abcdefghijklmnopqrstuvwxyz0123456789._-/")


def is_reserved(key: str) -> bool:
    if key in RESERVED_EXACT:
        return True
    return any(key.startswith(p) for p in RESERVED_PREFIXES)


def valid_label_key(key: str) -> tuple[bool, str]:
    if key == "":
        return False, "key must not be empty"
    if len(key.encode()) > MAX_KEY_LEN:
        return False, f"key must be <= {MAX_KEY_LEN} bytes (got {len(key.encode())})"
    for ch in key:
        if ch not in LABEL_KEY_CHARS:
            return False, f"key contains disallowed character {ch!r} (allowed: a-z 0-9 . _ - /)"
    return True, ""


def valid_label_value(value: str) -> tuple[bool, str]:
    if value == "":
        return False, "value must not be empty"
    if len(value.encode()) > MAX_VALUE_LEN:
        return False, f"value must be <= {MAX_VALUE_LEN} bytes (got {len(value.encode())})"
    for ch in value:
        if unicodedata.category(ch) == "Cf" or (ord(ch) < 0x20 or ord(ch) == 0x7f):
            return False, f"value contains a control or format character: {ch!r}"
    return True, ""


def matcher_safe(key: str) -> tuple[bool, str]:
    """Alertmanager matcher label-name grammar: [a-zA-Z_][a-zA-Z0-9_]*"""
    if key == "":
        return False, "key must not be empty"
    if not (key[0].isalpha() or key[0] == "_"):
        return False, f"first character {key[0]!r} must be a letter or underscore"
    for ch in key[1:]:
        if not (ch.isalnum() or ch == "_"):
            return False, (
                f"character {ch!r} is not valid in a matcher label name "
                "(only [a-zA-Z0-9_] allowed — note this is STRICTER than what "
                "SetCollectorLabel accepts for the key itself: '.', '-', '/' are "
                "legal label-key characters but NOT legal matcher label-name characters)"
            )
    return True, ""


def main(argv: list[str]) -> int:
    if len(argv) == 3 and argv[1] == "--matcher-safe":
        key = argv[2]
        key_lower = key.lower()
        if key_lower in BUILTIN_MATCHER_KEYS:
            print(f"OK: {key_lower!r} — matcher-safe (built-in, always populated, never gated by an org flag).")
            return 0
        ok, msg = matcher_safe(key)
        if is_reserved(key_lower):
            print(f"REJECT: {key!r} is reserved (id/os/alloy_version, or collector.*/shepherd.* prefix) and is "
                  "NEVER added to the matched-against label set by BuildCollectorLabels — cannot be used as a "
                  "matcher source at all, even though it may be visible elsewhere in the UI.")
            return 1
        print(("OK" if ok else "REJECT") + f": {key!r} — {'matcher-safe' if ok else msg}")
        return 0 if ok else 1

    if len(argv) != 3:
        print(__doc__)
        return 2

    key, value = argv[1], argv[2]
    key_lower = key.lower()

    problems = []
    ok_key, msg_key = valid_label_key(key_lower)
    if not ok_key:
        problems.append(f"key: {msg_key}")
    if is_reserved(key_lower):
        problems.append(
            "key: reserved for a built-in collector attribute "
            "(cluster, role, id, os, alloy_version, or a collector.*/shepherd.* prefix)"
        )
    ok_value, msg_value = valid_label_value(value)
    if not ok_value:
        problems.append(f"value: {msg_value}")

    if key != key_lower:
        print(f"NOTE: key will be lowercased to {key_lower!r} when set via the admin-label API "
              "(SetCollectorLabel lowercases before validating — an uppercase key as typed here "
              "is not itself a rejection reason, but if you're pasting this key into a matcher "
              "literally as typed, it won't match).")

    if problems:
        print("REJECT:")
        for p in problems:
            print(f"  - {p}")
        return 1

    ms_ok, ms_msg = matcher_safe(key_lower)
    print(f"OK: key={key_lower!r} value={value!r} is a valid admin label.")
    if ms_ok:
        print(f"    Also matcher-safe: usable directly as a pipeline matcher label name.")
    else:
        print(f"    WARNING: NOT matcher-safe ({ms_msg}) — this label cannot be referenced "
              "in a pipeline matcher even though it's a valid label.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
