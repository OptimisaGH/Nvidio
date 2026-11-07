# Security Policy

## Supported versions

Only the latest release of Nvidio receives security fixes.

| Version | Supported |
| ------- | --------- |
| 1.0.x   | Yes       |
| Alpha / older | No  |

## What Nvidio does

Nvidio runs with administrator rights because it changes Windows settings and GPU power limits. It:

- Reads and writes a small set of Windows registry values
- Changes the active power plan and pauses a few background services
- Runs NVIDIA's `nvidia-smi` tool to read GPU stats and set power limits
- Saves the original values in `C:\ProgramData\Nvidio\state.json` so changes can be undone

It does not connect to the internet, collect data, install drivers, or run in the background after you close the window.

## Reporting a vulnerability

Please report security problems privately, not in a public issue.

1. Go to the **Security** tab of this repository.
2. Click **Report a vulnerability**.
3. Describe what you found, how to reproduce it, and what you think the impact is.

I aim to reply within 7 days and to fix confirmed issues as soon as I reasonably can. Please give me a chance to release a fix before sharing details publicly. You are welcome to be credited in the release notes.

## What counts

Examples of issues worth reporting:

- A way for a non-administrator to make Nvidio change settings it shouldn't
- A way to alter Nvidio's saved state file to cause harm when Nvidio runs
- A setting that can't be undone with **Restore defaults** and leaves a system unstable
- A tampered or malicious build being distributed under the Nvidio name

Not security issues (please use the **Issues** tab instead):

- Settings that don't improve performance on your system
- Windows SmartScreen warnings, since the exe is not code-signed yet
- Power limits that your GPU or driver refuses

## Verifying your download

Only download Nvidio from the **Releases** page of this repository. Each file shows a SHA-256 checksum there. To check it, run this in PowerShell and compare the result:

```
Get-FileHash .\Nvidio.exe -Algorithm SHA256
```

Copies from other sites are not official and may be modified.
