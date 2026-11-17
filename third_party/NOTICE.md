# Third-party notices

## Ip2Region — offline IP data and XDB layout

Pinned source: https://github.com/lionsoul2014/ip2region/tree/92e6a788d17c3185c852ca983124c3fca280f992

Copyright 2022 The Ip2Region Authors. Apache-2.0 OR MIT; this project uses the Apache-2.0 option. Full license: licenses/ip2region-LICENSE.md. The XDB reader in internal/platform/ipregion.go adapts the upstream layout into an immutable-buffer reader. Official IPv4/IPv6 snapshots remain unmodified; hashes and build times are retained in the deployment manifests. No extra Go dependencies are introduced.

## Natural Earth — coarse geographic regions

Source: https://github.com/nvkelso/natural-earth-vector/blob/ca96624a56bd078437bca8184e78163e5039ad19/geojson/ne_50m_admin_1_states_provinces.geojson

Public domain, https://www.naturalearthdata.com/about/terms-of-use/ . A subset of administrative region polygons is used for coarse, buffered comparison only. No geographic accuracy or precise device location is implied. Attribution: Made with Natural Earth.

## OpenCelliD — cell tower positions

Source: https://opencellid.org (per-country file 460.csv.gz, downloaded monthly by cmd/cell-sync).

Data © OpenCelliD contributors, licensed under CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/). Used unmodified at runtime for risk checks; the data is not redistributed with the SDK.

## RootBeer 0.1.2 — optional Android collector

Copyright (C) 2015, Scott Alexander-Bown, Mat Rollings.

Project: https://github.com/scottyab/rootbeer

Artifact: com.scottyab:rootbeer-lib:0.1.2

License: Apache License, Version 2.0. Full text: licenses/rootbeer-LICENSE.txt.

The upstream artifact is unmodified. Device Trust Platform supplies a separate adapter in android/sdk-rootbeer. The base SDK and Go server do not depend on RootBeer.

## Vue and Vite

The console uses Vue under the MIT License. Full text: licenses/vue-LICENSE.txt.
Vite is the console build tool; its license and bundled dependency notices are retained in licenses/vite-LICENSE.md.

## xollvm v0.12.0 / LLVM22 — native build tool

Project: https://github.com/und3ath/xollvm/tree/v0.12.0

Apache License 2.0 with LLVM Exceptions and upstream component notices. Full text: licenses/xollvm-LICENSE.txt. Generates obfuscated native object code. Toolchain binaries are not redistributed. Build metrics and hashes are retained. Portable injected IR CPU attributes are removed before Android code generation; compiler sources are unmodified.
