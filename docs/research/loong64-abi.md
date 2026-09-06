# loong64 ABI and kernel on the target distributions

- **Date:** 2026-09-05
- **Ticket:** [headlesslab/wand#28](https://github.com/headlesslab/wand/issues/28) — _Establish which target loong64 builds run official Go binaries (new-world ABI, kernel)_
- **Parent:** [headlesslab/wand#1](https://github.com/headlesslab/wand/issues/1)
- **Scope:** facts only, each with a source. No decision is made here. Anything that could not be
  confirmed against a primary source is written as **not found**.
- **Background already established in [`domestic-platforms.md`](./domestic-platforms.md)** (§1.4,
  §1.5, §2) and not repeated here: the `golang` package versions in each distro's `loongarch64`
  repository, the availability of `linux-loong64` official tarballs (Go 1.21+), and the Go port's
  feature matrix.

## The question (verbatim from #28)

> ## Question
>
> Go's `linux/loong64` port targets the LoongArch new-world ABI (LP64D) and, per the Go 1.19 release notes, needs kernel >= 5.19; the same notes warn that "most existing commercial Linux distributions for LoongArch come with older kernels, with a historical incompatible system call ABI", where even statically linked binaries fail. Before wand promises cross-compiled `loong64` binaries, establish, for each target distribution with a `loongarch64` build:
>
> - **Kylin V10 SP3 server**, **Kylin V10 desktop 10.1**, **openEuler 22.03 LTS (GA)**, **openEuler 24.03 LTS (SP1+)**, and **UOS 20** if any public source exists: the kernel version shipped, old-world vs new-world ABI, and whether binaries built by the _official_ Go toolchain (`GOOS=linux GOARCH=loong64`) are reported to run there (distro docs, Loongson community wiki, the Go issue tracker, openEuler / Kylin bug trackers, Loongnix documentation on "新世界 / 旧世界").
> - Whether each distro's own `golang` package is upstream Go or a Loongson-patched fork (the `golang-1.15.7-*.a.ky10.loongarch64` and `golang-1.17.3-*.oe2203.loongarch64` packages predate upstream loong64 support in Go 1.19), which decides whether Go-1.21-level code compiles with the system Go at all.
> - Whether Go's documented kernel requirement (5.19) is a hard runtime requirement or a "tested on" statement (check the Go issue tracker and `src/runtime` for loong64 syscall usage).
>
> Output: one table per distribution with sources on a `research/` branch (`docs/research/loong64-abi.md`). Do not decide.

---

## Summary table

All package data read from the distributions' own repositories on 2026-09-05. "ELF interpreter" is
the dynamic loader path shipped by the distribution's `glibc` / `libc6` package on `loongarch64`;
per Loongson community documentation this is the decisive test of which world a system belongs to
(see §1).

| Distribution (loongarch64)   | Kernel shipped                                                                                                     | glibc / ELF interpreter                                                 | World / ABI                                                                                           | Official Go `linux/loong64` binaries run?                                                                                          | Distro `golang` package                                                                                                                                           |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Kylin V10 SP3 server**     | `4.19.90-52.22.v2207.a.ky10` (base) … `4.19.90-52.66` (updates)                                                    | 2.28 · `/lib64/ld.so.1`                                                 | **Old world (ABI 1.0)**                                                                               | **No** (Loongson: upstream Go "无法在 4.19 版本内核的操作系统上运行"; a Kylin V10 report exists on the Go tracker, though for SP1) | Go **1.15.7** + Kylin-authored `0001-Kylin-add-golang1.15.7-9-loong64-support.patch` → **patched fork**, not upstream                                             |
| **Kylin V10 desktop 10.1**   | `linux-image-4.19.0-loongson-3` = **4.19.167-rc5.lnd.1**                                                           | `libc6` 2.28-10.kylin.26k1 · `/lib64/ld.so.1` → `ld-2.28.so`            | **Old world (ABI 1.0)**                                                                               | **No** (same Loongson statement; `麒麟` named explicitly as ABI 1.0)                                                               | `golang-1.15` **1.15.6-1.lnd.9** — `.lnd` = Loongnix/Loongson revision → **Loongson-patched**, not upstream                                                       |
| **openEuler 22.03 LTS (GA)** | `5.10.0-60.116.0.143.oe2203`                                                                                       | 2.34 · `/lib64/ld-linux-loongarch-lp64d.so.1`                           | **New world (ABI 2.0)** — despite the 5.10 version number, Loongson states it is 5.19-UAPI-compatible | **Yes** — Loongson names openEuler 22.03 as an ABI 2.0 system on which the community (official) Go runs                            | Go **1.17.3** + a **99-patch Loongson LoongArch backport** (`loongarch64.tar.gz` / `loongarch64.conf`) → **patched fork** (upstream 1.17.3 has no loong64 at all) |
| **openEuler 24.03 LTS SP1+** | SP1 `6.6.0-72.0.0.76.oe2403sp1`; SP4 `6.6.0-159.4.2.153…oe2403sp4`                                                 | 2.38 · `/lib64/ld-linux-loongarch-lp64d.so.1`                           | **New world (ABI 2.0)** (kernel ≥ 5.19, new-world interpreter)                                        | **Not found** as a direct report; kernel and interpreter meet every documented condition                                           | Go **1.21.4** built from upstream `go1.21.4.src.tar.gz` with **no LoongArch backport series** — only small Loongson fixes → **essentially upstream**              |
| **UOS 20**                   | **not found** for `loongarch64` specifically. Uniontech states UOS 20 desktop ships a dual 4.19 / 5.10 kernel line | **not found** (repos return HTTP 401, see `domestic-platforms.md` §1.3) | **Old world (ABI 1.0)** — named as such by both Loongson and the community wiki                       | **No** (Loongson names `UOS` as ABI 1.0)                                                                                           | **not found**                                                                                                                                                     |

---

## 1. What "new world" means, and how it is decided

Loongson's own Go documentation (`docs.loongnix.cn`, page updated 2026-06-12) states the criterion
in terms of the kernel **UAPI**, not the kernel version string. Verbatim, question 5:

> 因 Linux 内核社区在 5.19 版本正式合入了对 LoongArch64 架构的支持，所以上游社区的 Loong64 上 Golang 对内核最小需求是 5.19，
> 这导致上游社区的 Loong64 上的 Golang 无法在 4.19 版本内核的操作系统上运行．这里发布的 ABI1.0 版本二进制是指可以运行在以４.19 内核 UAPI 为基础的操作系统上的程序，
> 如 Loongnix server 8.4, Loongnix 20，UOS，麒麟等；ABI2.0 版本指的是可以运行在以５.19 内核 UAPI 为基础的操作系统上的程序，
> 如 OpenEuler 22.03 (虽然 Openeuler 22.03 的内核版本为 5.10,但是兼容 5.19 内核的 UAPI, 所以 ABI2.0 版本的 Golang 也可以在 OpenEuler 22.03 上运行).

Translated in substance: because the LoongArch64 port landed in mainline Linux at 5.19, upstream Go
for loong64 has a minimum kernel requirement of 5.19, and it therefore **cannot run on a
4.19-kernel OS**. Loongson's "ABI 1.0" builds target systems based on the **4.19 kernel UAPI** —
named examples: **Loongnix server 8.4, Loongnix 20, UOS, 麒麟 (Kylin)**. "ABI 2.0" builds target
systems based on the **5.19 kernel UAPI** — named example: **openEuler 22.03**, and Loongson
explicitly notes that although openEuler 22.03's kernel is _numbered_ 5.10, it is compatible with
the 5.19 UAPI, so ABI 2.0 Go runs on it.

The same page, question 4:

> 从 Go1.21 开始，Golang 社区提供 LoongArch64 平台上的二进制，可以直接下载使用，需要注意的是 Golang 社区提供的二进制是 ABI2.0 的版本，只能在内核版本为 5.10 及以上的操作系统上运行.

i.e. the Go community's own loong64 binaries (Go 1.21+) are **ABI 2.0** builds.

Source: <http://docs.loongnix.cn/golang/faq.html>

The community reference site AREWELOONGYET (maintained by LoongArch kernel/toolchain contributors)
gives a matching, more operational definition:

> 如果符合以下任一条件，你就在用 旧世界：
>
> - 系统是麒麟 V10、Loongnix 20、UOS V20 其中之一
> - 内核版本以 4.19、5.4 或 5.10 开头
> - 有 WPS 用但没有安装过 libLoL 等旧世界兼容方案
>   如果一条都没中，你就在用 新世界。

and the decisive per-binary test:

> 可以使用 `file` 工具方便地检查一个二进制程序属于哪个世界。… 如果输出的行含有这些字样：
> `interpreter /lib64/ld.so.1, for GNU/Linux 4.15.0` 就表明这是一个旧世界程序。
> 相应地，如果输出的行含有这些字样：
> `interpreter /lib64/ld-linux-loongarch-lp64d.so.1, for GNU/Linux 5.19.0` 就表明这是一个新世界程序。

Its component version table:

> | 软件 | 旧世界版本 | 新世界版本 |
> | Linux | 4.19 | ≥ 5.19，常见 ≥ 6.1 |
> | glibc | 2.28 | ≥ 2.36 |
> | Go | 1.15、1.18、1.19 | ≥ 1.19 |

and its distribution lists:

> 目前已知的旧世界发行版（移植）有： 麒麟 (Kylin) V10 · Loongnix 20 · UOS V20
> 目前已知的新世界发行版（移植）有： ALT Linux · AOSC OS · CLFS · Debian · Fedora LoongArch Remix ·
> Gentoo · 麒麟 (Kylin) V11 · Loong Arch Linux · Loongnix 25 · Slackware · UOS V25 · Yongbao

Note that **openEuler appears in neither list** on that page; Loongson's own FAQ (quoted above) is
the source that places openEuler 22.03 in ABI 2.0.

The same page explains the Go-specific failure:

> 具体而言，Go 程序在异世界运行时，初始化过程中必须的一次 `rt_sigprocmask` 系统调用会由于它使用的 `NSIG`
> 常量定义与当前运行内核不同而失败，此时 Go 会故意访问一个非法地址直接崩溃。

Sources: <https://areweloongyet.com/docs/old-and-new-worlds/> ·
<https://areweloongyet.com/en/docs/old-and-new-worlds/>

**The `glibc` interpreter test applied to each target repository** (this document's own
verification, reading each distro's package payload / file list on 2026-09-05):

| Distribution                         | `glibc` / `libc6` version     | Interpreter shipped                                        | Verdict   |
| ------------------------------------ | ----------------------------- | ---------------------------------------------------------- | --------- |
| Kylin V10 SP3 server, loongarch64    | `glibc-2.28-12.p04.01.a.ky10` | `/lib64/ld.so.1`, `/lib64/ld-2.28.so`                      | old world |
| Kylin V10 desktop 10.1, loongarch64  | `libc6 2.28-10.kylin.26k1`    | `/lib64/ld.so.1` → `/lib/loongarch64-linux-gnu/ld-2.28.so` | old world |
| openEuler 22.03 LTS, loongarch64     | `glibc-2.34-122.oe2203`       | `/lib64/ld-linux-loongarch-lp64d.so.1`                     | new world |
| openEuler 24.03 LTS SP1, loongarch64 | `glibc-2.38-47.oe2403sp1`     | `/lib64/ld-linux-loongarch-lp64d.so.1`                     | new world |

Sources (repository metadata read directly):
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/repodata/> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/pool/glibc/libc6_2.28-10.kylin.26k1_loongarch64.deb> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-22.03-LTS/everything/loongarch64/repodata/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP1/everything/loongarch64/repodata/>

---

## 2. Kylin V10 SP3, server line (`update.cs2c.com.cn/NS/V10/V10SP3`)

**Kernel.** The `loongarch64` tree ships `kernel-4.19.90-52.22.v2207.a.ky10.loongarch64` in `base`;
the `updates` repository carries the same 4.19.90 line up to `kernel-4.19.90-52.66.v2207.a.ky10`.
There is no kernel package of any other version in either repository on this architecture.
**glibc is 2.28**, and it installs `/lib64/ld.so.1` and `/lib64/ld-2.28.so`.

**World.** Old world / ABI 1.0. Three independent statements agree: the kernel is 4.19 (Loongson's
own boundary), the interpreter is `/lib64/ld.so.1` (the old-world path), and both Loongson's Go FAQ
("如…UOS，麒麟等" under ABI 1.0) and AREWELOONGYET (麒麟 V10 listed as 旧世界) name Kylin V10
directly.

**Do official Go binaries run?** No. Loongson states upstream loong64 Go "无法在 4.19 版本内核的操作
系统上运行". The closest first-hand report on the Go issue tracker is
[golang/go#68867](https://github.com/golang/go/issues/68867), filed against **Kylin V10 SP1** on
LoongArch (`Linux dev1-pc 5.4.18-55-generic #44-KYLINOS SMP … loongarch64`) with official Go 1.19
`linux/loong64`:

> ```
> root@dev1-pc:~/test# go version
> 段错误 (核心已转储)
> …
> Program received signal SIGSEGV, Segmentation fault.
> runtime.rtsigprocmask () at …/src/runtime/sys_linux_loong64.s:360
> #1  0x000000000004d264 in runtime.sigprocmask (…) at …/src/runtime/os_linux.go:442
> ```

A Loongson Go contributor answered on that issue:

> The minimum required kernel version on Linux/long64 is 5.19, and I think [here](http://docs.loongnix.cn/golang/faq.html) is the answer you need.
> If you need Go that can run on Linux/loong64 low version kernels (such as 4.19, 5.4, etc.), please move [here](http://www.loongnix.cn/zh/toolchain/Golang/).

A report against **SP3 specifically** is **not found**; SP3's kernel (4.19.90) is older than SP1's
(5.4.18) in the report above, and both are below the boundary.

**Distro `golang` package: patched fork.** The newest package is `golang-1.15.7-60.p01.a.ky10`. Its
RPM changelog (read from the repository's `other.xml.gz`) records where loong64 support came from —
the base entry `1.15.7-9.p02`, by `wangshuo <wangshuo@kylinos.cn>`:

> - Type:update
> - ID:[TASK#78310]
> - SUG:NA
> - DESC:Add Patch9002:0001-Kylin-add-golang1.15.7-9-loong64-support.patch
> -      Modify spec, fix empty go-shared.list logical error

Upstream Go 1.15.7 has no `loong64` port whatsoever (the port landed in Go 1.19), so this package is
a **Kylin-patched fork** carrying an out-of-tree LoongArch backport. Every later revision through
`1.15.7-60.p01` is CVE backporting; the earlier `1.15.7-2` … `1.15.7-9` entries are openEuler
(`@huawei.com`) authors, showing the package's openEuler ancestry.

Sources:
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/Packages/> ·
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/updates/loongarch64/Packages/> ·
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/repodata/> ·
<https://github.com/golang/go/issues/68867> · <http://docs.loongnix.cn/golang/faq.html>

---

## 3. Kylin V10 desktop 10.1 (`archive.kylinos.cn/kylin/KYLIN-ALL`, suite `10.1`)

**Kernel.** The only kernel image in `main` for `loongarch64` is
`linux-image-4.19.0-loongson-3`, version **`4.19.167-rc5.lnd.1`**. The `.lnd` revision suffix marks
it as a Loongnix (Loongson) build. `libc6` is `2.28-10.kylin.26k1`.

**World.** Old world / ABI 1.0. Unpacking the shipped `libc6_2.28-10.kylin.26k1_loongarch64.deb`
gives the interpreter symlink:

```
./lib64/ld.so.1 -> /lib/loongarch64-linux-gnu/ld-2.28.so
```

which is the old-world path per AREWELOONGYET's test. Kernel 4.19 and glibc 2.28 match the
old-world column of that page's version table exactly.

**Do official Go binaries run?** No — same Loongson statement (`麒麟` under ABI 1.0). A report
naming the desktop edition specifically is **not found**.

**Distro `golang` package: Loongson-patched.** `golang-1.15` is `1.15.6-1.lnd.9`. The `.lnd`
revision is the Loongnix/Loongson downstream marker (the same suffix as the kernel image on this
architecture); the `Maintainer` field is inherited from Debian's Go Compiler Team, but Debian never
shipped a `loongarch64` build of `golang-1.15`, and upstream Go 1.15.6 has no `loong64` port. The
default `golang` metapackage on this architecture is `2:1.15~1.lnd.1`.

Sources:
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/main/binary-loongarch64/Packages.gz> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/pool/glibc/libc6_2.28-10.kylin.26k1_loongarch64.deb> ·
<https://areweloongyet.com/docs/old-and-new-worlds/>

---

## 4. openEuler 22.03 LTS (GA)

**Kernel.** `kernel-5.10.0-60.116.0.143.oe2203.loongarch64`, `glibc-2.34-122.oe2203.loongarch64`.
(The `loongarch64` tree exists only under 22.03-LTS GA `everything`; `update/loongarch64/` returns
404, and SP1–SP4 have no `loongarch64` tree — see `domestic-platforms.md` §1.4.)

**World: new world / ABI 2.0**, notwithstanding the 5.10 version number. Two lines of evidence:

1. Loongson states it directly: "ABI2.0 版本指的是可以运行在以５.19 内核 UAPI 为基础的操作系统上的程序，
   如 OpenEuler 22.03 (虽然 Openeuler 22.03 的内核版本为 5.10,但是兼容 5.19 内核的 UAPI, 所以 ABI2.0 版本的
   Golang 也可以在 OpenEuler 22.03 上运行)."
2. Its `glibc` ships `/lib64/ld-linux-loongarch-lp64d.so.1` — the new-world interpreter — and not
   `/lib64/ld.so.1`.

This is the one case in scope where the "kernel ≥ 5.19" shorthand gives the wrong answer and the
UAPI criterion gives the right one.

**Do official Go binaries run?** **Yes**, per the Loongson FAQ sentence quoted above — it is the
single named example of a system where ABI 2.0 (= community/official) Go runs. Note the caveat that
this is a Loongson statement about openEuler 22.03 as a platform; a separate independent
reproduction (Go tracker, openEuler tracker) was **not found**.

**Distro `golang` package: Loongson-patched fork.** `golang-1.17.3-*.oe2203.loongarch64`. The
`golang.spec` on the `openEuler-22.03-LTS` branch takes upstream `go1.17.3.src.tar.gz` as `Source0`
and then, only on this architecture, applies a Loongson patch bundle:

```spec
%ifarch loongarch64
%global gohostarch loong64
%endif
...
Source0:        https://dl.google.com/go/go1.17.3.src.tar.gz
%ifarch loongarch64
Source1:        loongarch64.tar.gz
Source2:        loongarch64.conf
Source3:        apply-patches
%endif
...
%prep
%autosetup -n go -p1
%ifarch loongarch64
cp %{SOURCE1} .
cp %{SOURCE2} .
cp %{SOURCE3} .
sh ./apply-patches
%endif
```

`loongarch64.conf` is the patch series list and contains **99 patches**, beginning
`0001-cmd-internal-sys-declare-loong64-arch.patch` and running through
`0017-runtime-bootstrap-for-linux-loong64-…`, `0036-syscall-add-syscall-support-for-linux-loong64…`,
`0090-syscall-update-linux-loong64-kernel-ABI-emulate-fsta…`,
`0092-cmd-asm-link-loong64-switch-to-LoongArch-ELF-psABI-v…`,
`0099-cmd-internal-objfile-add-loong64-disassembler-suppor…`. The spec changelog attributes them to
Loongson:

> \* Thu Dec 15 2022 chenguoqi \<chenguoqi@loongson.cn\> - 1.17.3-13
>
> - Add loongarch64 base support
>
> \* Thu Apr 20 2023 huangqiqi \<huangqiqi@loongson.cn\> - 1.17.3-17
>
> - Added support for shared, plugin, disassembly, etc. in long64, and fixed known bugs

So the system Go on openEuler 22.03 `loongarch64` is Go **1.17.3 language level** with a LoongArch
backport — it is not upstream Go, and it does not accept Go 1.21-level source.

Sources:
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-22.03-LTS/everything/loongarch64/Packages/> ·
<https://gitee.com/src-openeuler/golang/raw/openEuler-22.03-LTS/golang.spec> ·
<https://gitee.com/src-openeuler/golang/raw/openEuler-22.03-LTS/loongarch64.conf> ·
<http://docs.loongnix.cn/golang/faq.html>

---

## 5. openEuler 24.03 LTS (SP1 and later)

**Kernel.** SP1: `kernel-6.6.0-72.0.0.76.oe2403sp1.loongarch64`, `glibc-2.38-47.oe2403sp1`.
SP4: `kernel-6.6.0-159.4.2.153.20260625.abca3926d80c.oe2403sp4.loongarch64`, `glibc-2.38-107`.

**World: new world / ABI 2.0.** Kernel 6.6 is above the 5.19 mainline-merge boundary, glibc 2.38 is
above the ≥ 2.36 new-world figure, and `glibc` ships `/lib64/ld-linux-loongarch-lp64d.so.1`. All
three criteria from §1 agree.

**Do official Go binaries run?** **Not found** as an explicit report. No statement naming openEuler
24.03 was located on the Go issue tracker, the openEuler forum, `docs.loongnix.cn`, or
AREWELOONGYET. What is established is that every documented condition for running ABI 2.0 binaries
is met, and that Loongson's own bar (a 5.19-UAPI kernel) is cleared with margin.

**Distro `golang` package: essentially upstream.** `golang-1.21.4-*.oe2403sp1.loongarch64`. The
`golang.spec` on the `openEuler-24.03-LTS-SP1` branch takes `Source0: https://dl.google.com/go/go1.21.4.src.tar.gz`
and has **no** `loongarch64.tar.gz` / `loongarch64.conf` / `apply-patches` sources and no
`loong`-named patch in the series — the only architecture-specific line is:

```spec
%ifarch loongarch64
%global gohostarch loong64
%endif
```

which is correct because upstream Go 1.21.4 already carries the loong64 port. The Loongson-authored
changelog entries on this branch are incremental fixes rather than a port:

> \* Thu Apr 18 2024 Huang Yang \<huangyang@loongson.cn\> - 1.21.4-8
>
> - enable external_linker and cgo on loongarch64
>
> \* Tue Mar 26 2024 Wenlong Zhang \<zhangwenlong@loongson.cn\> - 1.21.4-4
>
> - fix build error for loongarch64

Per `domestic-platforms.md` §1.4, SP3/SP4 additionally ship a parallel `golang-1.24` (Go 1.24.6),
including on `loongarch64` in SP4.

Sources:
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP1/everything/loongarch64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP4/everything/loongarch64/Packages/> ·
<https://gitee.com/src-openeuler/golang/raw/openEuler-24.03-LTS-SP1/golang.spec>

---

## 6. UOS 20

**Kernel: not found for `loongarch64`.** Uniontech's own package repositories are credential-gated
(HTTP 401; see `domestic-platforms.md` §1.3), so no kernel package version could be read from a
first-party repository. Uniontech's public FAQ states that UOS 20 desktop offers a **dual kernel
line, 4.19 and 5.10**, both described as upstream LTS kernels — but that page does not break the
figure down by architecture, so it is not evidence about the `loongarch64` build specifically. Both
numbers are below 5.19 in any case.

**World: old world / ABI 1.0.** Named as such by two sources:

- Loongson's Go FAQ lists `UOS` among ABI 1.0 systems ("如 Loongnix server 8.4, Loongnix 20，UOS，麒麟等").
- AREWELOONGYET lists `UOS V20` under 旧世界发行版, and `UOS V25` under 新世界发行版.

**Do official Go binaries run?** No, per the Loongson statement (UOS named under ABI 1.0). The same
FAQ's question 9 is written specifically about this class of system:

> 问题 9: Loongnix、UOS、keylin 等 ABI1.0 的系统上有些 Go 的项目中执行 Go 命令时段错误.
> 在一些 Go 实现的项目中在 go.mod 中指定了 Go 编译器的最低版本需求，如果当前系统中的 Go 版本低于 go.mod 中指定的版本，
> 则自动从 go.dev 下载 go.mod 中指定的 go 版本并使用该版本编译当前项目，但是因为从 go.dev 下载的二进制只能在 ABI2.0 系统上运行，所以出现段错误。
> 解决方法是从 Loongnix 站点下载 go.mod 指定的版本.

That is, on an ABI 1.0 system, `GOTOOLCHAIN` auto-download from go.dev fetches an ABI 2.0 toolchain
and the build segfaults; the documented workaround is to install the matching version from
Loongson's site instead. (The worked example given is building `github.com/cli/cli`, whose `go.mod`
requires go1.24.3 on a system with go1.23.9.)

**Distro `golang` package: not found** — the repositories are inaccessible.

Sources: <http://docs.loongnix.cn/golang/faq.html> ·
<https://areweloongyet.com/docs/old-and-new-worlds/> ·
<https://faq.uniontech.com/solution/5cb3/39f8/05fd>

---

## 7. Is Go's 5.19 kernel requirement hard, or a "tested on" statement?

**Both documented forms, read verbatim.**

The Go 1.19 release notes state it as a requirement with a consequence:

> Go 1.19 adds support for the Loongson 64-bit architecture LoongArch on Linux (`GOOS=linux`,
> `GOARCH=loong64`). The implemented ABI is LP64D. Minimum kernel version supported is 5.19.
>
> Note that most existing commercial Linux distributions for LoongArch come with older kernels, with
> a historical incompatible system call ABI. Compiled binaries will not work on these systems, even
> if statically linked. Users on such unsupported systems are limited to the distribution-provided
> Go package.

The Go wiki phrases the same fact more loosely:

> For loong64, kernel 5.19 and later versions work fine.

(The wiki's general Linux minimum is "For Go 1.24 and later: Kernel version 3.2 or later.")

**What the source code shows.** There is **no kernel version check** in the loong64 path. Reading
`src/runtime` and `src/syscall` at `master` on 2026-09-05:

- `src/runtime/os_linux_generic.go` (built for loong64) defines the signal-set shape:

  ```go
  const (
      _SS_DISABLE  = 2
      _NSIG        = 65
      ...
  )

  // It's hard to tease out exactly how big a Sigset is, but
  // rt_sigprocmask crashes if we get it wrong, so if binaries
  // are running, this is right.
  type sigset [2]uint32
  ```

- `src/runtime/os_linux.go` passes that size straight to the kernel during startup:

  ```go
  func sigprocmask(how int32, new, old *sigset) {
      rtsigprocmask(how, new, old, int32(unsafe.Sizeof(*new)))
  }
  ```

- `src/runtime/sys_linux_loong64.s` makes the call and, on **any** error return, executes a
  deliberate illegal write rather than returning:

  ```asm
  // func rtsigprocmask(how int32, new, old *sigset, size int32)
  TEXT runtime·rtsigprocmask<ABIInternal>(SB),NOSPLIT,$0
      MOVV	$SYS_rt_sigprocmask, R11
      SYSCALL
      MOVW	$-4096, R5
      BGEU	R5, R4, 2(PC)
      MOVV	R0, 0xf1(R0)	// crash
      RET
  ```

The old-world LoongArch kernel uses `NSIG = 128` (a 16-byte sigset) where upstream uses `NSIG = 64`
(the 8-byte `sigset [2]uint32` above), so `rt_sigprocmask` returns `EINVAL` and the runtime crashes
at address `0xf1` before `main`. This is corroborated by the two Go tracker reports: in
[golang/go#55130](https://github.com/golang/go/issues/55130) the reporter's `dmesg` reads

> `do_page_fault(): sending SIGSEGV to go for invalid write access to 00000000000000f1`

— literally the `0xf1` from that instruction — for a **statically linked** cross-compiled binary
(`statically linked, Go BuildID=…`), and in [golang/go#68867](https://github.com/golang/go/issues/68867)
the gdb backtrace stops in `runtime.rtsigprocmask` at `sys_linux_loong64.s:360`. A Go team member
replied on #55130:

> See https://go.dev/doc/go1.19#loong64 … You'll need to update the kernel, or use
> distribution-provided Go package.

- `src/syscall/syscall_linux_loong64.go` further shows the port targets only the modern generic
  syscall set: `Stat`, `Lstat` and `Fstatat` are implemented via `statx` (there is no `fstat` /
  `newfstatat` in the upstream LoongArch UAPI), plus `SYS_RENAMEAT2`, `SYS_PSELECT6`,
  `_SYS_clone3 = 435`, `_SYS_faccessat2 = 439`, `_SYS_fchmodat2 = 452`. Loongson's own backport
  series carries a corresponding patch, `0090-syscall-update-linux-loong64-kernel-ABI-emulate-fsta…`,
  for the other direction.

**Conclusion.** The requirement is **hard at runtime, but it is a requirement on the kernel's
LoongArch UAPI, not on the version string**. Concretely:

1. It is not a "tested on" note. The failure on an old-world kernel is deterministic, happens during
   runtime initialisation before `main`, affects statically linked binaries, and is an intentional
   crash written into the assembly — not a soft degradation. Go performs **no** kernel-version
   detection and has no fallback path for loong64.
2. The number "5.19" is shorthand for "the LoongArch UAPI that was merged into mainline Linux at
   5.19". A kernel _numbered_ below 5.19 that carries that UAPI does run official binaries —
   openEuler 22.03's 5.10 kernel is the documented example (§4). Conversely, nothing at runtime
   reads `uname`, so a kernel numbered ≥ 5.19 that somehow kept the old UAPI would still fail. The
   `glibc` interpreter path (§1) is the reliable proxy, not the version number.

Sources: <https://go.dev/doc/go1.19> · <https://go.dev/wiki/MinimumRequirements> ·
<https://github.com/golang/go/blob/master/src/runtime/os_linux_generic.go> ·
<https://github.com/golang/go/blob/master/src/runtime/os_linux.go> ·
<https://github.com/golang/go/blob/master/src/runtime/sys_linux_loong64.s> ·
<https://github.com/golang/go/blob/master/src/syscall/syscall_linux_loong64.go> ·
<https://github.com/golang/go/issues/55130> · <https://github.com/golang/go/issues/68867>

---

## What could not be established

- **UOS 20 `loongarch64` kernel version, glibc version and `golang` package.** Uniontech's
  repositories return HTTP 401. Only the world classification (old world / ABI 1.0) is sourced, from
  Loongson and AREWELOONGYET; the dual 4.19 / 5.10 kernel figure comes from a Uniontech FAQ page
  that does not break down by architecture.
- **A first-hand report of official Go `linux/loong64` binaries running on openEuler 24.03 LTS.**
  Not found on the Go issue tracker, the openEuler forum, `docs.loongnix.cn` or AREWELOONGYET. Only
  the criteria (kernel 6.6, new-world interpreter) are established.
- **A first-hand report against Kylin V10 SP3 server specifically.** The Go tracker report
  ([#68867](https://github.com/golang/go/issues/68867)) is against Kylin V10 SP1 on LoongArch
  (kernel 5.4.18); SP3's `loongarch64` kernel is 4.19.90.
- **An independent (non-Loongson) reproduction that official Go binaries run on openEuler 22.03
  LTS.** The claim rests on Loongson's Go FAQ.
- **An openEuler-side statement classifying its LoongArch port as new world / ABI 2.0.** The
  openEuler announcement page for the 22.03 LTS LoongArch release could not be fetched
  (TLS handshake failure from this network on both `www.openeuler.org` and `repo.openeuler.org`);
  the classification here rests on Loongson's FAQ plus the repository evidence in §1.

---

## Sources

**Loongson / community documentation**

- Loongson Go FAQ (ABI 1.0 vs ABI 2.0, kernel UAPI, distribution names, `GOTOOLCHAIN` segfault),
  page updated 2026-06-12 — <http://docs.loongnix.cn/golang/faq.html>
- Loongson LoongArch Go toolchain downloads — <http://www.loongnix.cn/zh/toolchain/Golang/>
- AREWELOONGYET, "旧世界与新世界" — <https://areweloongyet.com/docs/old-and-new-worlds/> ·
  English: <https://areweloongyet.com/en/docs/old-and-new-worlds/>
- Loongnix documentation root — <https://docs.loongnix.cn/>

**Go project**

- Go 1.19 release notes, loong64 section — <https://go.dev/doc/go1.19>
- Go wiki, Minimum Requirements — <https://go.dev/wiki/MinimumRequirements>
- `src/runtime/os_linux_generic.go` — <https://github.com/golang/go/blob/master/src/runtime/os_linux_generic.go>
- `src/runtime/os_linux.go` — <https://github.com/golang/go/blob/master/src/runtime/os_linux.go>
- `src/runtime/sys_linux_loong64.s` — <https://github.com/golang/go/blob/master/src/runtime/sys_linux_loong64.s>
- `src/syscall/syscall_linux_loong64.go` — <https://github.com/golang/go/blob/master/src/syscall/syscall_linux_loong64.go>
- golang/go#55130, statically linked cross-compiled loong64 binary segfaults —
  <https://github.com/golang/go/issues/55130>
- golang/go#68867, Kylin V10 SP1 loongarch64, `sys_linux_loong64.s:360` —
  <https://github.com/golang/go/issues/68867>
- golang/go#46229, `all: port to linux/loong64` — <https://github.com/golang/go/issues/46229>

**Distribution repositories and packaging**

- Kylin V10 SP3 server, loongarch64 base packages and repodata —
  <https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/Packages/> ·
  <https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/repodata/>
- Kylin V10 SP3 server, loongarch64 updates —
  <https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/updates/loongarch64/Packages/> ·
  <https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/updates/loongarch64/repodata/>
- Kylin V10 desktop 10.1, loongarch64 —
  <http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/main/binary-loongarch64/Packages.gz> ·
  <http://archive.kylinos.cn/kylin/KYLIN-ALL/pool/glibc/libc6_2.28-10.kylin.26k1_loongarch64.deb>
- openEuler 22.03 LTS, loongarch64 —
  <https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-22.03-LTS/everything/loongarch64/Packages/> ·
  <https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-22.03-LTS/everything/loongarch64/repodata/>
- openEuler 24.03 LTS SP1 / SP4, loongarch64 —
  <https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP1/everything/loongarch64/Packages/> ·
  <https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP4/everything/loongarch64/Packages/>
- openEuler `golang` packaging, 22.03-LTS branch —
  <https://gitee.com/src-openeuler/golang/raw/openEuler-22.03-LTS/golang.spec> ·
  <https://gitee.com/src-openeuler/golang/raw/openEuler-22.03-LTS/loongarch64.conf>
- openEuler `golang` packaging, 24.03-LTS-SP1 branch —
  <https://gitee.com/src-openeuler/golang/raw/openEuler-24.03-LTS-SP1/golang.spec>
- Uniontech UOS 20 dual-kernel FAQ — <https://faq.uniontech.com/solution/5cb3/39f8/05fd>
