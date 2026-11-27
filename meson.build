project(
  'sd-report-collector',
  meson_version : '>= 0.61.0',
  default_options : [
                  'prefix=/usr',
                  'sysconfdir=/etc',
                  'localstatedir=/var'],
  license : ['Apache-2.0'],
  version : '0.8',
)


go = find_program('go')

source_root = meson.project_source_root()

vendor_libdir = get_option('vendor_libdir')

go_env = environment()
go_env.set('GOFLAGS', '')
# Builds run with build_always_stale, so 'meson install' re-invokes 'go
# build' even when already compiled. Under 'sudo meson install', ninja runs
# with privileges dropped to the invoking user but $HOME still set to
# root's, so Go's default $HOME-derived cache path is unwritable. Pin
# GOCACHE inside the build directory instead, which the invoking user owns
# regardless of $HOME.
go_env.set('GOCACHE', meson.project_build_root() / 'go-cache')

binaries = {
  'sd-report-certs': get_option('bindir'),
  'sd-report-collector': vendor_libdir,
  'sd-report-dashboard': vendor_libdir,
  'sd-report-influxdb': vendor_libdir,
}

built_binaries = {}
foreach name, install_subdir : binaries
  built_binaries += {name: custom_target(name,
    output: name,
    command: [go, '-C', source_root, 'build', '-buildmode=pie',
              '-o', meson.current_build_dir() / name, './cmd/' + name],
    env: go_env,
    build_always_stale: true,
    build_by_default: true,
    install: true,
    install_dir: install_subdir,
    install_mode: 'rwxr-xr-x',
  )}
endforeach

test('go test', go,
  args: ['-C', source_root, 'test', './...'],
  env: go_env,
)

run_target('fmt', command: [go, '-C', source_root, 'fmt', './...'])
run_target('tidy', command: [go, '-C', source_root, 'mod', 'tidy'])
run_target('update', command: [go, '-C', source_root, 'get', '-v', '-u', './...'])

subdir('dist')
