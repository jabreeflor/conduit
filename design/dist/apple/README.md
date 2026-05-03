# ConduitDesign (Swift Package)

SwiftUI tokens for the Conduit design system. Generated from
[`design/tokens.yaml`](https://github.com/jabreeflor/conduit/blob/main/design/tokens.yaml)
by `cmd/design-tokens`.

## Install

```swift
.package(url: "https://github.com/jabreeflor/conduit.git", from: "0.1.0")
```

Then depend on the `ConduitDesign` library product in your target.

## Usage

```swift
import SwiftUI
import ConduitDesign

struct PrimaryButton: View {
    var body: some View {
        Text("Run")
            .foregroundStyle(ConduitColor.fgOnAccent)
            .background(ConduitColor.bgAccent)
    }
}
```

`Tokens.swift` exposes `ConduitColor` and `ConduitFont` helpers. Mode
switching is handled by the host app via `colorScheme` and the high-
contrast accessibility setting.
