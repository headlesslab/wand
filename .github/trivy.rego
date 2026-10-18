# The one finding every Trivy step of the Gate and the Nightly lets through:
# the kernel's own vulnerabilities, reported against linux-libc-dev.
#
# That package is the kernel's user-space headers, which build-essential pulls
# into the development image for -race and the launcher's fixture binaries.
# It carries no kernel: a container runs on its host's, so none of these is
# reachable from inside the image, and Ubuntu publishes a batch of them every
# few weeks, each of which kept the Nightly red until the next release
# republished :dev (#142). Every other package, the C library beside it
# included, is still held to CRITICAL.
package trivy

import rego.v1

default ignore := false

ignore if input.PkgName == "linux-libc-dev"
