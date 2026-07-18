require 'json'

package = JSON.parse(File.read(File.join(__dir__, 'package.json')))

Pod::Spec.new do |s|
  s.name = 'AeroShutterTcpSocket'
  s.version = package['version']
  s.summary = package['description']
  s.license = 'MIT'
  s.homepage = 'https://github.com/subhashraveendran/aero-shutter'
  s.author = 'Subhash Raveendran'
  s.source = { git: 'https://github.com/subhashraveendran/aero-shutter.git', tag: s.version.to_s }
  s.source_files = 'ios/Plugin/**/*.{swift,h,m,c,cc,mm,cpp}'
  s.ios.deployment_target = '13.0'
  # NEHotspotConfiguration / NEHotspotNetwork for in-app Wi-Fi joining.
  s.frameworks = 'NetworkExtension', 'SystemConfiguration'
  s.dependency 'Capacitor'
  s.swift_version = '5.1'
end
