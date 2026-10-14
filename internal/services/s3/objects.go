package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbenardo/aws-tui/internal/ui"
)

// maxObjects caps how many entries a single prefix listing loads.
const maxObjects = 10000

type entry struct {
	key      string
	isDir    bool
	size     int64
	modified time.Time
	class    string
}

func (e entry) name(prefix string) string { return strings.TrimPrefix(e.key, prefix) }

// objectsView browses one "folder" (prefix) of a bucket.
type objectsView struct {
	c      *client
	bucket bucket
	prefix string
}

func newObjectsView(c *client, b bucket, prefix string) ui.View {
	ov := &objectsView{c: c, bucket: b, prefix: prefix}

	title := b.name
	if prefix != "" {
		title = path.Base(prefix) + "/"
	}

	return ui.NewTable(ui.TableConfig{
		Title: title,
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Size", Width: 10},
			{Title: "Last modified", Width: 19},
			{Title: "Storage class", Width: 14},
		},
		Load:     ov.load,
		OnSelect: ov.open,
		Actions: []ui.Action{
			ui.NewAction("d", "download", ov.withFile(func(e entry) tea.Cmd { return promptDownload(c, ov.bucket, e.key) })),
			ui.NewAction("u", "upload", func(*ui.Row) tea.Cmd { return ov.promptUpload() }),
			ui.NewAction("n", "new folder", func(*ui.Row) tea.Cmd { return ov.promptNewFolder() }),
			ui.NewAction("x", "delete", ov.withFile(ov.confirmDelete)),
		},
		Tags: func(ctx context.Context, r ui.Row) (map[string]string, error) {
			e := r.Data.(entry)
			if e.isDir {
				return nil, errors.New("folders have no tags; open an object to see its tags")
			}
			return objectTags(ctx, c, ov.bucket, e.key)
		},
		EmptyText: "This folder is empty. Press u to upload a file.",
	})
}

func (ov *objectsView) uri(key string) string { return fmt.Sprintf("s3://%s/%s", ov.bucket.name, key) }

func (ov *objectsView) api(ctx context.Context) (*s3.Client, error) {
	api, _, err := ov.c.forBucket(ctx, ov.bucket)
	return api, err
}

func (ov *objectsView) load(ctx context.Context) ([]ui.Row, error) {
	api, err := ov.api(ctx)
	if err != nil {
		return nil, err
	}

	var dirs, files []entry
	p := s3.NewListObjectsV2Paginator(api, &s3.ListObjectsV2Input{
		Bucket:    aws.String(ov.bucket.name),
		Prefix:    aws.String(ov.prefix),
		Delimiter: aws.String("/"),
	})
	for p.HasMorePages() && len(dirs)+len(files) < maxObjects {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, cp := range page.CommonPrefixes {
			dirs = append(dirs, entry{key: aws.ToString(cp.Prefix), isDir: true})
		}
		for _, o := range page.Contents {
			k := aws.ToString(o.Key)
			if k == ov.prefix { // folder placeholder object
				continue
			}
			files = append(files, entry{
				key:      k,
				size:     aws.ToInt64(o.Size),
				modified: aws.ToTime(o.LastModified),
				class:    string(o.StorageClass),
			})
		}
	}

	rows := make([]ui.Row, 0, len(dirs)+len(files))
	for _, d := range dirs {
		rows = append(rows, ui.Row{ID: ov.uri(d.key), Cells: []string{"📁 " + d.name(ov.prefix), "-", "-", "-"}, Data: d})
	}
	for _, f := range files {
		rows = append(rows, ui.Row{
			ID:    ov.uri(f.key),
			Cells: []string{"  " + f.name(ov.prefix), ui.HumanBytes(f.size), ui.FormatTime(f.modified), f.class},
			Data:  f,
		})
	}
	return rows, nil
}

func (ov *objectsView) open(r ui.Row) tea.Cmd {
	e := r.Data.(entry)
	if e.isDir {
		return ui.Push(newObjectsView(ov.c, ov.bucket, e.key))
	}
	return ui.Push(newObjectView(ov.c, ov.bucket, e.key))
}

// withFile adapts an action so it only runs on files.
func (ov *objectsView) withFile(fn func(e entry) tea.Cmd) func(*ui.Row) tea.Cmd {
	return func(r *ui.Row) tea.Cmd {
		if r == nil {
			return nil
		}
		e := r.Data.(entry)
		if e.isDir {
			return ui.Error(errors.New("this action only works on files"))
		}
		return fn(e)
	}
}

func (ov *objectsView) confirmDelete(e entry) tea.Cmd {
	uri := fmt.Sprintf("s3://%s/%s", ov.bucket.name, e.key)
	return ui.Confirm("Delete "+uri+"?", ui.Task("Deleting "+uri+"…", true, func(ctx context.Context) (string, error) {
		api, err := ov.api(ctx)
		if err != nil {
			return "", err
		}
		if _, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(ov.bucket.name), Key: aws.String(e.key)}); err != nil {
			return "", err
		}
		return "Deleted " + uri, nil
	}))
}

func (ov *objectsView) promptUpload() tea.Cmd {
	return ui.Prompt("Upload local file:", "", func(local string) tea.Cmd {
		if local == "" {
			return nil
		}
		local = expandHome(local)
		key := ov.prefix + filepath.Base(local)
		uri := fmt.Sprintf("s3://%s/%s", ov.bucket.name, key)
		return ui.Task("Uploading "+local+" to "+uri+"…", true, func(ctx context.Context) (string, error) {
			f, err := os.Open(local)
			if err != nil {
				return "", err
			}
			defer f.Close()
			api, err := ov.api(ctx)
			if err != nil {
				return "", err
			}
			if _, err := api.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(ov.bucket.name), Key: aws.String(key), Body: f}); err != nil {
				return "", err
			}
			return "Uploaded " + uri, nil
		})
	})
}

func (ov *objectsView) promptNewFolder() tea.Cmd {
	return ui.Prompt("New folder name:", "", func(name string) tea.Cmd {
		name = strings.Trim(name, "/")
		if name == "" {
			return nil
		}
		key := ov.prefix + name + "/"
		return ui.Task("Creating folder "+key+"…", true, func(ctx context.Context) (string, error) {
			api, err := ov.api(ctx)
			if err != nil {
				return "", err
			}
			if _, err := api.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(ov.bucket.name), Key: aws.String(key), Body: strings.NewReader("")}); err != nil {
				return "", err
			}
			return "Folder " + key + " created", nil
		})
	})
}

// promptDownload asks for a destination and downloads the object there.
func promptDownload(c *client, b bucket, key string) tea.Cmd {
	return ui.Prompt("Download to:", "./"+path.Base(key), func(dest string) tea.Cmd {
		if dest == "" {
			return nil
		}
		dest = expandHome(dest)
		return ui.Task("Downloading "+key+"…", false, func(ctx context.Context) (string, error) {
			api, _, err := c.forBucket(ctx, b)
			if err != nil {
				return "", err
			}
			n, err := download(ctx, api, b.name, key, dest)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Downloaded %s to %s", ui.HumanBytes(n), dest), nil
		})
	})
}

func download(ctx context.Context, api *s3.Client, bucket, key, dest string) (int64, error) {
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		dest = filepath.Join(dest, path.Base(key))
	}
	out, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return 0, err
	}
	defer out.Body.Close()

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, out.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, os.Rename(tmp, dest)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func objectTags(ctx context.Context, c *client, b bucket, key string) (map[string]string, error) {
	api, _, err := c.forBucket(ctx, b)
	if err != nil {
		return nil, err
	}
	out, err := api.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: aws.String(b.name), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	return tagMap(out.TagSet), nil
}
