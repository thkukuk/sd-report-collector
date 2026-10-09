project(
  'sd-report-collector',
  meson_version : '>= 0.61.0',
  default_options : [
                  'prefix=/usr',
                  'sysconfdir=/etc',
                  'localstatedir=/var'],
  license : ['Apache-2.0'],
  version : '0.0',
)


go = find_program('go')

source_root = meson.project_source_root()

vendor_libdir = get_option('vendor_libdir')

go_env = environment()
go_env.set('GOFLAGS', '')

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
