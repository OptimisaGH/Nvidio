> [!CAUTION]
> **NOTICE: Forking this repository is not allowed under any circumstance. The repository was recently changed to ARR (All Rights Reserved)** 

# Nvidio

Nvidio is a lightweight optimisation utility for NVIDIA GeForce GTX 950 and newer graphics cards on Windows. It shows live GPU stats, lets you lower heat and noise with power-limit profiles, and applies a few Windows settings that help games run smoothly. Every change can be undone.

Built for NVIDIA GPUs only. Intel, AMD and Apple Silicon graphics are not supported.

[![Download Nvidio](https://img.shields.io/badge/Download-Nvidio-76b900?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/OptimisaGH/Nvidio/releases)

# Features

- **Live GPU monitor**: temperature, load, power draw, core clock, fan speed and VRAM use.
- **Thermal profiles**: Performance (stock), Balanced (90% power limit) and Cool & quiet (80%) to reduce heat and noise.
- **Windows optimisations**: High performance power plan, Game Mode, background game capture off, lighter desktop compositing, and hardware GPU scheduling (GTX 10-series or newer, restart required).
- **Fully reversible**: every change is recorded and can be undone with one click.
- **Lightweight**: no background service and no startup entry. Nvidio closes when you close its window.

Results vary by system and game. Cooler temperatures are the most consistent benefit. Frame rate gains are usually strong and depend on what is holding your system back.

# System Requirements

**Minimum**
- NVIDIA GeForce GTX 950 or newer, with the NVIDIA driver installed
- Windows 10 (64-bit)
- Administrator rights (Nvidio asks when it starts)

**Recommended**
- NVIDIA GeForce GTX 1650 or newer
- Windows 11 (64-bit)

The download is only a few MB.

# Installation

1. Open the **Releases** tab on the right side of this page.
2. Download `Nvidio.exe` from the latest release.
3. Run it and click **Yes** on the administrator prompt. If Windows shows "Windows protected your PC", click **More info**, then **Run anyway**. The file is not code-signed yet.
4. Pick a thermal profile and turn on the optimisations you want. Restart Windows only if Nvidio tells you to.

# Known limitations

- Power limit changes reset when you restart your PC.
- Some laptop and older GPUs don't allow power limit changes. Nvidio shows the driver's error if so.

# License

All Rights Reserved

Copyright © 2026 OptimisaGH

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

# Support & Bug Tracking

If something doesn't work:

- Open the **Issues** tab at the top of this repository.
- Open a new ticket with your GPU model, Windows version, and what you saw.
