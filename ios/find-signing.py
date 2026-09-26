#!/usr/bin/env python3
"""Finds how to sign Cliff Crack for a device (used by build-ios.sh).

Looks through the provisioning profiles Xcode has downloaded for one that
fits the bundle id (or, with none given, one made for Cliff Crack), and
through the keychain for a signing identity whose certificate that profile
includes. Prints shell assignments: identity=<SHA-1> profile=<path>
bundle_id=<id>. Exits non-zero with advice when there's no match.
"""
import datetime
import glob
import hashlib
import os
import plistlib
import re
import shlex
import subprocess
import sys

want = sys.argv[1] if len(sys.argv) > 1 and sys.argv[1] else None

identities = set(re.findall(
    r"^\s*\d+\)\s+([0-9A-F]{40})\s",
    subprocess.run(["security", "find-identity", "-v", "-p", "codesigning"],
                   capture_output=True, text=True).stdout, re.M))

dirs = ["~/Library/Developer/Xcode/UserData/Provisioning Profiles",
        "~/Library/MobileDevice/Provisioning Profiles"]
candidates = []
for d in dirs:
    for path in glob.glob(os.path.join(os.path.expanduser(d), "*.mobileprovision")):
        raw = subprocess.run(["security", "cms", "-D", "-i", path], capture_output=True).stdout
        try:
            p = plistlib.loads(raw)
        except Exception:
            continue
        if p["ExpirationDate"] < datetime.datetime.now():
            continue
        team, _, app = p["Entitlements"]["application-identifier"].partition(".")
        certs = {hashlib.sha1(c).hexdigest().upper() for c in p.get("DeveloperCertificates", [])}
        usable = certs & identities
        if not usable:
            continue
        if want:
            if app == want:
                rank = 0
            elif app == "*":
                rank = 1
            else:
                continue
            bundle = want
        else:
            if "cliffcrack" in app.lower() and app != "*":
                rank, bundle = 0, app
            else:
                continue
        candidates.append((rank, -p["ExpirationDate"].timestamp(), sorted(usable)[0], path, bundle))

if not candidates:
    print("No provisioning profile and signing identity for "
          + (want or "a Cliff Crack bundle id") + " were found.\n"
          "Set one up once in Xcode (see README, 'On your iPhone'): sign in with your Apple ID,\n"
          "make an empty iOS app with that bundle id, and run it on your phone.", file=sys.stderr)
    sys.exit(1)

_, _, identity, profile, bundle = min(candidates)
print(f"identity={shlex.quote(identity)} profile={shlex.quote(profile)} bundle_id={shlex.quote(bundle)}")
