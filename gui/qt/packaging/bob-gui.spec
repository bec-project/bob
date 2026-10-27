# bob's desktop app for RHEL 9, with bob-manager and the OpenCode it runs in its sandbox. RHEL 9 has
# no Qt 6.12 (EPEL 9 ships 6.10), so the app carries its own copy of Qt's official binaries in
# /opt/bob-gui; ci/build_qt_rpm.sh installs them for the build and passes their prefix as
# qt_prefix. It also builds bob-manager and fetches the pinned OpenCode release (opencode_version)
# as sources. The app finds bob-manager beside itself, and bob-manager finds OpenCode in libexec.
%global bob_version %{?version_override}%{!?version_override:0.0.0}
%global app_dir /opt/bob-gui
# ci/build_qt_rpm.sh pins OpenCode and defines this
%{!?opencode_version:%global opencode_version 0}
# Qt's binaries come stripped and without debug information; bob-manager is built without them
%global debug_package %{nil}
# OpenCode is a Bun executable whose program follows the ELF image: stripping would cut it off
%global __strip /bin/true
# the bundled Qt serves this app only: it provides nothing to other packages and needs nothing
# from the system's Qt
%global __provides_exclude_from ^%{app_dir}/.*$
%global __requires_exclude ^lib(Qt6|icu).*$

Name:           bob-gui
Version:        %{bob_version}
Release:        1%{?dist}
Summary:        Desktop app for bob, the OpenCode assistant in a sandbox
# The repository has no license file yet; the bundled Qt is LGPL-3.0-only, OpenCode MIT
License:        Proprietary AND LGPL-3.0-only AND MIT
URL:            https://github.com/bec-project/bob
Source0:        %{name}-%{version}.tar.gz
Source1:        bob-manager
Source2:        opencode-%{opencode_version}-linux-x64.tar.gz
Source3:        OPENCODE-LICENSE

BuildRequires:  cmake >= 3.22
BuildRequires:  gcc-c++
BuildRequires:  ninja-build
BuildRequires:  desktop-file-utils
BuildRequires:  libappstream-glib
# what Qt's binaries are built and tested against
BuildRequires:  mesa-libGL-devel
BuildRequires:  mesa-libEGL-devel
BuildRequires:  libxkbcommon-devel
BuildRequires:  fontconfig
BuildRequires:  dejavu-sans-fonts
# bob-manager runs OpenCode in a bubblewrap sandbox, as container_t where SELinux is on
Requires:       bubblewrap
Requires:       container-selinux

%description
A chat window for the OpenCode server that bob runs in a bubblewrap sandbox. The app
starts the user's bob through bob-manager, or joins the one that runs, and holds it while open.
It brings its own Qt 6.12, bob-manager and OpenCode %{opencode_version} in %{app_dir}.

%prep
%autosetup -n %{name}-%{version}

%build
cmake -S . -B build -G Ninja \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_PREFIX_PATH=%{qt_prefix} \
    -DCMAKE_INSTALL_PREFIX=%{app_dir} \
    -DBOB_VERSION=%{version} \
    -DBOB_SOURCE_DEFAULTS=OFF \
    -DBOB_BUNDLE_QT=ON
cmake --build build

%install
DESTDIR=%{buildroot} cmake --install build
# shared objects executable, so rpm reads what the bundled Qt needs from the system (xcb, GL, ...)
find %{buildroot}%{app_dir} -name '*.so*' -type f -exec chmod 0755 {} +
# the launcher, menu entry and icons where the desktop looks for them
mkdir -p %{buildroot}%{_bindir} %{buildroot}%{_datadir}
# a script, not a symlink: Qt looks for its plugins and qt.conf beside the path it was started as
printf '#!/bin/sh\nexec %{app_dir}/bin/bob-gui "$@"\n' > %{buildroot}%{_bindir}/bob-gui
chmod 0755 %{buildroot}%{_bindir}/bob-gui
# bob-manager beside the app, where it looks first, and on the PATH for the command line
install -m 0755 %{SOURCE1} %{buildroot}%{app_dir}/bin/bob-manager
ln -s %{app_dir}/bin/bob-manager %{buildroot}%{_bindir}/bob-manager
# OpenCode where bob-manager looks for it: libexec beside its bin
mkdir -p %{buildroot}%{app_dir}/libexec
tar -xzf %{SOURCE2} -C %{buildroot}%{app_dir}/libexec opencode
chmod 0755 %{buildroot}%{app_dir}/libexec/opencode
install -D -m 0644 %{SOURCE3} %{buildroot}%{app_dir}/licenses/opencode/LICENSE
for part in applications metainfo icons; do
    mv %{buildroot}%{app_dir}/share/$part %{buildroot}%{_datadir}/
done
rmdir %{buildroot}%{app_dir}/share

%check
desktop-file-validate %{buildroot}%{_datadir}/applications/ch.psi.bob.desktop
appstream-util validate-relax --nonet %{buildroot}%{_metainfodir}/ch.psi.bob.metainfo.xml
(cd build && QT_QPA_PLATFORM=offscreen ctest --output-on-failure)

%files
%{app_dir}
%{_bindir}/bob-gui
%{_bindir}/bob-manager
%{_datadir}/applications/ch.psi.bob.desktop
%{_metainfodir}/ch.psi.bob.metainfo.xml
%{_datadir}/icons/hicolor/*/apps/ch.psi.bob.png
