# Third-party license maintenance

Evela's original code/assets use the root MIT `LICENSE`. Dependencies, fonts,
native components and CCP data keep their own terms. `THIRD_PARTY_NOTICES.txt`
is generated, embedded in the executable, exposed by **Settings → Licenses…**,
and attached separately to releases. The ZIP continues to contain only the exe.

Run `python tools/licenses/generate.py --check` to verify the committed notice
against every direct/indirect requirement in `go.mod`. After dependency changes,
review compatibility and nested asset/header notices, then regenerate with
`python tools/licenses/generate.py`. The generator is a notice collector, not an
automatic license-compatibility determination. Review source headers when adding
native dependencies: a module's root license may not cover all its components.

The reviewed dependency set uses MIT, ISC, BSD, Apache-2.0 and zlib terms;
go-text's BSD option is used where it offers BSD or Unlicense. Fyne's bundled
Inter and other fonts retain OFL/DejaVu/Bitstream/Arev terms. The collector keeps
upstream license, copyright, author, patent and NOTICE files outside test/example
directories, including Fyne's font notices, HarfBuzz, GLFW and ICO subcomponents.
It also preserves the additional notices from go-gl's generated OpenGL bindings,
Khronos platform headers, and GLFW's Wine-derived DirectInput/XInput headers.
Unicode data used by Go and its text/segmentation/shaping packages retains the
Unicode notices in `data/`: the historical Unicode-DFS-2016 text from SPDX and
Unicode's current V3 text from https://www.unicode.org/license.txt.

## Reviewed native license snapshots

- `native/MinGW-w64-runtime.txt`: verbatim from the MSYS2 mingw-w64 CRT package,
  `mingw64/share/licenses/crt/COPYING.MinGW-w64-runtime.txt`.
- `native/Winpthreads.txt`: from the MSYS2 libwinpthread package's `COPYING`.
- `native/GPL-3.0.txt` and `native/GCC-Runtime-Exception-3.1.txt`: from the
  MSYS2 gcc-libs package, `COPYING3` and `COPYING.RUNTIME`. The exception permits
  eligible compiled combinations under chosen terms; the compiler executable
  and unrelated GCC libraries are not shipped. These baseline files came from
  the local MSYS2 GCC 14.2.0 installation.
- `graphics/LGPL-2.1.txt`: verbatim LGPL 2.1 text from gcc-libs `COPYING.LIB`,
  included for GLFW's Wine-derived interface headers. See the header notices and
  LGPL section 5; no Wine library is linked by the Windows build.
- `graphics/OPUBL-1.0.txt` and `graphics/SGI-B-2.0.txt`: standard license texts
  from the [SPDX license-list-data repository](https://github.com/spdx/license-list-data/tree/main/text),
  for the historical Khronos/SGI notices retained in the pinned go-gl headers.
  No optional Open Publication restrictions are elected in those binding headers.
- `graphics/GLAD.txt`: the MIT/Khronos notices from
  https://github.com/Dav1dde/glad/blob/v2.0.0/LICENSE, retained for GLFW's
  bundled GLAD-generated Vulkan interface declarations.

The runtime exception is documented by the [Free Software Foundation](https://www.gnu.org/licenses/gcc-exception-3.1.html).
MinGW source/licensing is maintained at [mingw-w64](https://www.mingw-w64.org/license/).
The exact GLFW header source is available through the version pinned in `go.mod`.
The code uses only its declarations, constants and small macros; builds that add
substantial LGPL inline code or link new libraries require a fresh review.

CI first runs `--check` using these reviewed snapshots, then regenerates with
`--native-root <mingw-prefix>/share/licenses` before compiling. This embeds the
runtime notices from the compiler installation actually used in each release.
When changing compilers, refresh the baseline snapshots and check the executable's
DLL imports as well as statically incorporated libraries. Current Windows builds
import only Windows system DLLs; the operating system's DLLs are not redistributed.

## CCP materials

`npc_data.json.gz`, `ore_data.json` and `hull_data.json` remain CCP-derived game
data, outside the MIT grant. `overview.txt` supplies attribution and links to
CCP's [Developer License Agreement](https://developers.eveonline.com/license-agreement).
Downstream redistribution/use of those materials must satisfy CCP's terms;
an MIT license for Evela's code does not grant a license to CCP game data or marks.
Do not describe CCP data or third-party fonts as MIT-licensed Evela content.
