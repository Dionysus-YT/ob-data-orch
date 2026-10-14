package agenttelemetry

import (
	"archive/zip"
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ob-data-orch/internal/nodefacts"
	"ob-data-orch/internal/outputpath"
)

const metadataLimit = 64 << 10

// InstalledVersions 只读取本机固化 Java 的 release 和工具 lib 中含固定主类的 JAR 元数据。
// 不启动进程、不执行脚本、不按文件名猜版本；缺失、歧义和超限保持未知，独立于目录空间采样。
func (c *Collector) InstalledVersions() *nodefacts.RuntimeVersions {
	if c == nil || c.configuration == nil {
		return nil
	}
	configuration, err := c.configuration.RuntimeConfiguration()
	if err != nil || configuration.Digest == "" {
		return nil
	}
	versions := &nodefacts.RuntimeVersions{}
	javaPath, javaOK := outputpath.LocalFilesystemPath(configuration.Platform, configuration.JavaPath)
	toolHome, toolOK := outputpath.LocalFilesystemPath(configuration.Platform, configuration.ToolHome)
	if javaOK {
		versions.Java = javaInstalledVersion(javaPath)
	}
	if toolOK {
		versions.OBLoader, versions.OBDumper = toolInstalledVersions(toolHome)
	}
	return versions
}

func javaInstalledVersion(javaPath string) *nodefacts.InstalledVersion {
	resolved, err := filepath.EvalSymlinks(javaPath)
	if err != nil || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return nil
	}
	javaHome := filepath.Dir(filepath.Dir(resolved))
	content, err := readMetadataFile(filepath.Join(javaHome, "release"))
	// Linux 的 java 符号链接可能指向 JDK 内的 jre/bin/java；只回退到固定父安装目录。
	if err != nil && filepath.Base(javaHome) == "jre" {
		content, err = readMetadataFile(filepath.Join(filepath.Dir(javaHome), "release"))
	}
	if err != nil {
		return nil
	}
	var result *nodefacts.InstalledVersion
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key == "JAVA_VERSION" {
			if result != nil {
				return nil
			}
			value = strings.Trim(value, `"`)
			if nodefacts.ValidVersion(value) {
				result = &nodefacts.InstalledVersion{Version: value, Source: "JAVA_RELEASE"}
			} else {
				return nil
			}
		}
	}
	return result
}

func readMetadataFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > metadataLimit {
		return nil, os.ErrInvalid
	}
	content, err := io.ReadAll(io.LimitReader(file, metadataLimit+1))
	if len(content) > metadataLimit {
		return nil, os.ErrInvalid
	}
	return content, err
}

func toolInstalledVersions(toolHome string) (*nodefacts.InstalledVersion, *nodefacts.InstalledVersion) {
	files, err := os.ReadDir(filepath.Join(toolHome, "lib"))
	if err != nil || len(files) > 512 {
		return nil, nil
	}
	var loader, dumper *nodefacts.InstalledVersion
	loaderConflict, dumperConflict := false, false
	for _, file := range files {
		if file.IsDir() || file.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(file.Name(), ".jar") {
			continue
		}
		path := filepath.Join(toolHome, "lib", file.Name())
		info, err := file.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 512<<20 {
			continue
		}
		archive, err := zip.OpenReader(path)
		if err != nil {
			continue
		}
		if len(archive.File) > 32768 {
			_ = archive.Close()
			continue
		}
		hasLoader, hasDumper := false, false
		for _, entry := range archive.File {
			hasLoader = hasLoader || entry.Name == "com/oceanbase/tools/loaddump/cmd/Obloader.class"
			hasDumper = hasDumper || entry.Name == "com/oceanbase/tools/loaddump/cmd/Obdumper.class"
		}
		if hasLoader || hasDumper {
			version := jarInstalledVersion(&archive.Reader)
			if hasLoader {
				loader, loaderConflict = mergeVersion(loader, version, loaderConflict)
			}
			if hasDumper {
				dumper, dumperConflict = mergeVersion(dumper, version, dumperConflict)
			}
		}
		_ = archive.Close()
	}
	if loaderConflict {
		loader = nil
	}
	if dumperConflict {
		dumper = nil
	}
	return loader, dumper
}

func mergeVersion(previous, current *nodefacts.InstalledVersion, conflict bool) (*nodefacts.InstalledVersion, bool) {
	// 含同一固定主类但缺版本的第二个包也具有歧义，不能只挑能读取版本的那个包。
	if current == nil {
		return previous, true
	}
	if previous != nil && previous.Version != current.Version {
		return previous, true
	}
	return current, conflict
}

func jarInstalledVersion(archive *zip.Reader) *nodefacts.InstalledVersion {
	for _, candidate := range []struct{ path, key, source string }{
		{"META-INF/MANIFEST.MF", "Implementation-Version", "JAR_MANIFEST"},
		{"META-INF/maven/com.oceanbase/ob-loader-dumper/pom.properties", "version", "JAR_POM"},
	} {
		for _, entry := range archive.File {
			if entry.Name != candidate.path || entry.UncompressedSize64 > metadataLimit {
				continue
			}
			reader, err := entry.Open()
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(io.LimitReader(reader, metadataLimit+1))
			value := ""
			for scanner.Scan() {
				separator := ":"
				if candidate.source == "JAR_POM" {
					separator = "="
				}
				key, candidateValue, found := strings.Cut(scanner.Text(), separator)
				if found && strings.TrimSpace(key) == candidate.key {
					if value != "" {
						value = ""
						break
					}
					value = strings.TrimSpace(candidateValue)
				}
			}
			err = scanner.Err()
			_ = reader.Close()
			if err == nil && nodefacts.ValidVersion(value) {
				return &nodefacts.InstalledVersion{Version: value, Source: candidate.source}
			}
		}
	}
	return nil
}
