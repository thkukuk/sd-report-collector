systemd = dependency('systemd', required: false)
if systemd.found()
  unitdir = systemd.get_variable(pkgconfig: 'systemd_system_unit_dir')
  tmpfilesdir = systemd.get_variable(pkgconfig: 'tmpfiles_dir')
else
  unitdir = get_option('prefix') / 'lib/systemd/system'
  tmpfilesdir = get_option('prefix') / 'lib/tmpfiles.d'
endif

install_data('sd-report-collector.conf',
  install_dir: vendor_libdir,
  install_mode: 'rw-r--r--',
)

install_data('describe.v263',
  install_dir: vendor_libdir,
  install_mode: 'rw-r--r--',
)

install_data('sd-report-collector.service',
  install_dir: unitdir,
  install_mode: 'rw-r--r--',
)

install_data('sd-report-collector-node.service',
  install_dir: unitdir,
  install_mode: 'rw-r--r--',
)

install_data('sd-report-collector-node.timer',
  install_dir: unitdir,
  install_mode: 'rw-r--r--',
)

install_data('tmpfiles.d/sd-report-collector.conf',
  install_dir: tmpfilesdir,
  install_mode: 'rw-r--r--',
)

install_data('tmpfiles.d/sd-report-collector-node.conf',
  install_dir: tmpfilesdir,
  install_mode: 'rw-r--r--',
)
