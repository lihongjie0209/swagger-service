package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/lihongjie0209/swagger-service/internal/config"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const annotationPrefix = "platform.swagger/"

func StartKubernetesDiscovery(lc fx.Lifecycle, cfg config.Config, registry *Registry, logger *slog.Logger) error {
	if !cfg.Aggregation.Kubernetes.Enabled {
		return nil
	}
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("load in-cluster Kubernetes config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	namespace := cfg.Aggregation.Kubernetes.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}
	selector := cfg.Aggregation.Kubernetes.LabelSelector
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = selector
			return client.CoreV1().Services(namespace).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = selector
			return client.CoreV1().Services(namespace).Watch(ctx, options)
		},
	}
	informer := cache.NewSharedIndexInformer(lw, &corev1.Service{}, cfg.Aggregation.Kubernetes.ResyncPeriod, cache.Indexers{})
	reconcile := func() {
		values := make([]Source, 0)
		for _, object := range informer.GetStore().List() {
			service, ok := object.(*corev1.Service)
			if !ok {
				continue
			}
			if source, ok := sourceFromService(service); ok {
				values = append(values, source)
			}
		}
		registry.ReplaceKubernetes(values)
	}
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{AddFunc: func(any) { reconcile() }, UpdateFunc: func(any, any) { reconcile() }, DeleteFunc: func(any) { reconcile() }}); err != nil {
		return fmt.Errorf("register Kubernetes discovery handler: %w", err)
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		runCtx, stop := context.WithCancel(context.Background())
		cancel = stop
		go informer.Run(runCtx.Done())
		if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
			stop()
			return errorsNewCacheSync()
		}
		reconcile()
		logger.Info("Kubernetes Swagger discovery started", "namespace", namespace, "selector", selector)
		return nil
	}, OnStop: func(context.Context) error {
		if cancel != nil {
			cancel()
		}
		return nil
	}})
	return nil
}

func sourceFromService(service *corev1.Service) (Source, bool) {
	annotations := service.GetAnnotations()
	if !strings.EqualFold(annotations[annotationPrefix+"enabled"], "true") {
		return Source{}, false
	}
	portValue := strings.TrimSpace(annotations[annotationPrefix+"port"])
	var port int32
	for _, candidate := range service.Spec.Ports {
		if (portValue == "" && (candidate.Name == "http" || len(service.Spec.Ports) == 1)) || candidate.Name == portValue || strconv.Itoa(int(candidate.Port)) == portValue {
			port = candidate.Port
			break
		}
	}
	if port == 0 {
		return Source{}, false
	}
	name := strings.TrimSpace(annotations[annotationPrefix+"name"])
	if name == "" {
		name = service.Namespace + "--" + service.Name
	}
	title := strings.TrimSpace(annotations[annotationPrefix+"title"])
	if title == "" {
		title = service.Name
	}
	path := strings.TrimSpace(annotations[annotationPrefix+"path"])
	if path == "" {
		path = "/swagger/doc.json"
	}
	if !strings.HasPrefix(path, "/") {
		return Source{}, false
	}
	scheme := strings.TrimSpace(annotations[annotationPrefix+"scheme"])
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return Source{}, false
	}
	return Source{Name: name, Title: title, URL: fmt.Sprintf("%s://%s.%s.svc:%d%s", scheme, service.Name, service.Namespace, port, path), Origin: "kubernetes"}, true
}

func errorsNewCacheSync() error { return fmt.Errorf("synchronize Kubernetes Service informer cache") }

var Module = fx.Module("catalog", fx.Provide(NewRegistry), fx.Invoke(StartKubernetesDiscovery))
