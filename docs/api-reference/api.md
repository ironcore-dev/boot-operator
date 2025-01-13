<p>Packages:</p>
<ul>
<li>
<a href="#metal.ironcore.dev%2fv1alpha1">metal.ironcore.dev/v1alpha1</a>
</li>
</ul>
<h2 id="metal.ironcore.dev/v1alpha1">metal.ironcore.dev/v1alpha1</h2>
<div>
<p>Package v1alpha1 contains API Schema definitions for the settings.gardener.cloud API group</p>
</div>
Resource Types:
<ul><li>
<a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfig">HTTPBootConfig</a>
</li><li>
<a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfig">IPXEBootConfig</a>
</li></ul>
<h3 id="metal.ironcore.dev/v1alpha1.HTTPBootConfig">HTTPBootConfig
</h3>
<div>
<p>HTTPBootConfig is the Schema for the httpbootconfigs API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
metal.ironcore.dev/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>HTTPBootConfig</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfigSpec">
HTTPBootConfigSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier (UUID) of the server.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the secret containing Ignition configuration.</p>
</td>
</tr>
<tr>
<td>
<code>systemIPs</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>SystemIPs is a list of IP addresses assigned to the server.</p>
</td>
</tr>
<tr>
<td>
<code>ukiURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>UKIURL is the URL where the UKI (Unified Kernel Image) is hosted.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfigStatus">
HTTPBootConfigStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.IPXEBootConfig">IPXEBootConfig
</h3>
<div>
<p>IPXEBootConfig is the Schema for the ipxebootconfigs API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
metal.ironcore.dev/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>IPXEBootConfig</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfigSpec">
IPXEBootConfigSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier (UUID) of the server.</p>
</td>
</tr>
<tr>
<td>
<code>systemIPs</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>SystemIPs is a list of IP addresses assigned to the server.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image is deprecated and will be removed.</p>
</td>
</tr>
<tr>
<td>
<code>kernelURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>KernelURL is the URL where the kernel of the OS is hosted, eg. the URL to the Kernel layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>initrdURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>InitrdURL is the URL where the Initrd (initial RAM disk) of the OS is hosted, eg. the URL to the Initrd layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>squashfsURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>SquashfsURL is the URL where the Squashfs of the OS is hosted, eg.  the URL to the Squashfs layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>ipxeServerURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>IPXEServerURL is deprecated and will be removed.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the secret containing the Ignition configuration.</p>
</td>
</tr>
<tr>
<td>
<code>ipxeScriptSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IPXEScriptSecretRef is a reference to the secret containing the custom IPXE script.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfigStatus">
IPXEBootConfigStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.HTTPBootConfigSpec">HTTPBootConfigSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfig">HTTPBootConfig</a>)
</p>
<div>
<p>HTTPBootConfigSpec defines the desired state of HTTPBootConfig</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier (UUID) of the server.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the secret containing Ignition configuration.</p>
</td>
</tr>
<tr>
<td>
<code>systemIPs</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>SystemIPs is a list of IP addresses assigned to the server.</p>
</td>
</tr>
<tr>
<td>
<code>ukiURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>UKIURL is the URL where the UKI (Unified Kernel Image) is hosted.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.HTTPBootConfigState">HTTPBootConfigState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfigStatus">HTTPBootConfigStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Error&#34;</p></td>
<td><p>HTTPBootConfigStateError indicates that an error occurred while processing the HTTPBootConfig.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>HTTPBootConfigStatePending indicates that the HTTPBootConfig has not been processed yet.</p>
</td>
</tr><tr><td><p>&#34;Ready&#34;</p></td>
<td><p>HTTPBootConfigStateReady indicates that the HTTPBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.HTTPBootConfigStatus">HTTPBootConfigStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfig">HTTPBootConfig</a>)
</p>
<div>
<p>HTTPBootConfigStatus defines the observed state of HTTPBootConfig</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.HTTPBootConfigState">
HTTPBootConfigState
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.IPXEBootConfigSpec">IPXEBootConfigSpec
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfig">IPXEBootConfig</a>)
</p>
<div>
<p>IPXEBootConfigSpec defines the desired state of IPXEBootConfig</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>systemUUID</code><br/>
<em>
string
</em>
</td>
<td>
<p>SystemUUID is the unique identifier (UUID) of the server.</p>
</td>
</tr>
<tr>
<td>
<code>systemIPs</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>SystemIPs is a list of IP addresses assigned to the server.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image is deprecated and will be removed.</p>
</td>
</tr>
<tr>
<td>
<code>kernelURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>KernelURL is the URL where the kernel of the OS is hosted, eg. the URL to the Kernel layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>initrdURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>InitrdURL is the URL where the Initrd (initial RAM disk) of the OS is hosted, eg. the URL to the Initrd layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>squashfsURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>SquashfsURL is the URL where the Squashfs of the OS is hosted, eg.  the URL to the Squashfs layer of the OS OCI image.</p>
</td>
</tr>
<tr>
<td>
<code>ipxeServerURL</code><br/>
<em>
string
</em>
</td>
<td>
<p>IPXEServerURL is deprecated and will be removed.</p>
</td>
</tr>
<tr>
<td>
<code>ignitionSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IgnitionSecretRef is a reference to the secret containing the Ignition configuration.</p>
</td>
</tr>
<tr>
<td>
<code>ipxeScriptSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core">
Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<p>IPXEScriptSecretRef is a reference to the secret containing the custom IPXE script.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.IPXEBootConfigState">IPXEBootConfigState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfigStatus">IPXEBootConfigStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Error&#34;</p></td>
<td><p>IPXEBootConfigStateError indicates that an error occurred while processing the IPXEBootConfig.</p>
</td>
</tr><tr><td><p>&#34;Pending&#34;</p></td>
<td><p>IPXEBootConfigStatePending indicates that the IPXEBootConfig has not been processed yet.</p>
</td>
</tr><tr><td><p>&#34;Ready&#34;</p></td>
<td><p>IPXEBootConfigStateReady indicates that the IPXEBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.</p>
</td>
</tr></tbody>
</table>
<h3 id="metal.ironcore.dev/v1alpha1.IPXEBootConfigStatus">IPXEBootConfigStatus
</h3>
<p>
(<em>Appears on:</em><a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfig">IPXEBootConfig</a>)
</p>
<div>
<p>IPXEBootConfigStatus defines the observed state of IPXEBootConfig</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#metal.ironcore.dev/v1alpha1.IPXEBootConfigState">
IPXEBootConfigState
</a>
</em>
</td>
<td>
<p>Important: Run &ldquo;make&rdquo; to regenerate code after modifying this file</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
</em></p>
