package agenttelemetry

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/agentwire"
)

func writeVersionJar(t *testing.T, path string, content map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, body := range content {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func Test安装版本来自固定文件而不执行文件或从名称猜测(t *testing.T) {
	root := t.TempDir()
	javaHome, toolHome := filepath.Join(root, "java"), filepath.Join(root, "tool")
	for _, directory := range []string{filepath.Join(javaHome, "bin"), filepath.Join(toolHome, "lib")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	java := filepath.Join(javaHome, "bin", "java.exe")
	if err := os.WriteFile(java, []byte("synthetic non-executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(javaHome, "release"), []byte("JAVA_VERSION=\"1.8.0_491\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeVersionJar(t, filepath.Join(toolHome, "lib", "synthetic.jar"), map[string]string{
		"com/oceanbase/tools/loaddump/cmd/Obloader.class": "synthetic", "com/oceanbase/tools/loaddump/cmd/Obdumper.class": "synthetic",
		"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\nImplementation-Version: 4.3.5-RELEASE\n",
	})
	writeVersionJar(t, filepath.Join(toolHome, "lib", "ob-loader-dumper-99.99.jar"), map[string]string{"META-INF/MANIFEST.MF": "Implementation-Version: 99.99\n"})
	platform := "LINUX_AMD64"
	if filepath.VolumeName(root) != "" {
		platform = "WINDOWS_AMD64"
	}
	collector := NewCollector(runtimeConfigurationStub{configuration: agentwire.RuntimeConfiguration{Platform: platform, ToolHome: toolHome, JavaPath: java, Digest: strings.Repeat("a", 64)}})
	versions := collector.InstalledVersions()
	if versions == nil || versions.Java == nil || versions.Java.Version != "1.8.0_491" || versions.OBLoader == nil || versions.OBDumper == nil || versions.OBDumper.Version != "4.3.5-RELEASE" {
		t.Fatal("固定安装元数据未进入采集链")
	}
	writeVersionJar(t, filepath.Join(toolHome, "lib", "conflict.jar"), map[string]string{"com/oceanbase/tools/loaddump/cmd/Obdumper.class": "synthetic", "META-INF/MANIFEST.MF": "Implementation-Version: 4.3.6\n"})
	versions = collector.InstalledVersions()
	if versions.OBDumper != nil || versions.OBLoader == nil {
		t.Fatal("多个主类包的冲突未保持未知")
	}
	if err := os.WriteFile(filepath.Join(javaHome, "release"), []byte("JAVA_VERSION=\"1.8.0_491\"\nJAVA_VERSION=\"1.8.0_492\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if collector.InstalledVersions().Java != nil {
		t.Fatal("重复 Java 版本声明没有保持未知")
	}
}

func Test缺失非法超限和无主类版本不能补造(t *testing.T) {
	root := t.TempDir()
	if javaInstalledVersion(filepath.Join(root, "bin", "java")) != nil {
		t.Fatal("缺安装却生成版本")
	}
	for _, body := range []string{"Implementation-Version: /private/path\n", strings.Repeat("x", metadataLimit+1), "Implementation-Version: 4.3.5\nImplementation-Version: 4.3.6\n", ""} {
		jar := filepath.Join(root, "synthetic.jar")
		writeVersionJar(t, jar, map[string]string{"META-INF/MANIFEST.MF": body})
		archive, err := zip.OpenReader(jar)
		if err != nil {
			t.Fatal(err)
		}
		version := jarInstalledVersion(&archive.Reader)
		_ = archive.Close()
		if version != nil {
			t.Fatal("缺失或非法元数据生成版本")
		}
	}
	jar := filepath.Join(root, "pom.jar")
	writeVersionJar(t, jar, map[string]string{"META-INF/maven/com.oceanbase/ob-loader-dumper/pom.properties": "version=4.3.5-RELEASE\n"})
	archive, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if version := jarInstalledVersion(&archive.Reader); version == nil || version.Source != "JAR_POM" {
		t.Fatal("固定 Maven 元数据没有回退读取")
	}
}
