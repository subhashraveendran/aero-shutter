#import <Foundation/Foundation.h>
#import <Capacitor/Capacitor.h>

// Registers the plugin and its methods with the Capacitor bridge.
CAP_PLUGIN(TcpSocketPlugin, "TcpSocket",
           CAP_PLUGIN_METHOD(connect, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(write, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(close, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(getWifiInfo, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(getNetworkCapabilities, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(joinWifi, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(currentWifi, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(scanWifi, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(leaveWifi, CAPPluginReturnPromise);
)
