package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"path/filepath"

	"github.com/google/uuid"

	// "k8s.io/apimachinery/pkg/api/errors"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {

	clientset := getClientSet()

	initCluster(clientset)
	jobId := uuid.New()

	createJob(clientset, jobId.String(), "ls")
    /*
	stream, err := getJobLogs(clientset, jobId.String())
	if err != nil {
		panic(err.Error())
	}
	defer stream.Close()

	for {
		buf := make([]byte, 2000)
		numBytes, err := stream.Read(buf)
		if numBytes == 0 {
			continue
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Println(string(buf[:numBytes]))
		time.Sleep(time.Second)
	}
    */
}

func getClientSet() *kubernetes.Clientset {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func printPods(clientset *kubernetes.Clientset) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	for _, pod := range pods.Items {
		fmt.Printf("Pod : %-60s\t Namespace : %s\n", pod.Name, pod.Namespace)
	}
}

func initCluster(clientset *kubernetes.Clientset) {
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	found := false
	for _, namespace := range namespaces.Items {
		if namespace.Name == "scheduler" {
			found = true
			break
		}
	}
	if !found {
		newNamespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scheduler",
			},
		}
		_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), newNamespace, metav1.CreateOptions{})
		if err != nil {
			panic(err.Error())
		}
	}
}

func printJobs(clientset *kubernetes.Clientset) {
	jobs, err := clientset.BatchV1().Jobs("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d jobs in the cluster\n", len(jobs.Items))

	for _, job := range jobs.Items {
		fmt.Printf("Pod : %-60s\t Namespace : %s\n", job.Name, job.Namespace)
	}
}

func createJob(clientset *kubernetes.Clientset, name string, command string) {
	var backoffLimit int32 = 0
	newJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "scheduler",
		},
		Spec: batchv1.JobSpec{
			Template: apiv1.PodTemplateSpec{
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:    name,
							Image:   "jonlabelle/network-tools:latest",
							Command: strings.Split(command, " "),
						},
					},
					RestartPolicy: apiv1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backoffLimit,
		},
	}
	_, err := clientset.BatchV1().Jobs("scheduler").Create(context.TODO(), newJob, metav1.CreateOptions{})
	if err != nil {
		panic(err.Error())
	}
}

type TerminatedStream struct {
	stream io.ReadCloser
	stop   <-chan interface{}
}

func NewTerminatedStream(stream io.ReadCloser, closer func(chan<- interface{})) TerminatedStream {
	stopChannel := make(chan interface{})
	tStream := TerminatedStream{
		stream: stream,
		stop:   stopChannel,
	}
	go closer(stopChannel)
	go func() {
		<-stopChannel
		tStream.Close()
	}()
	return tStream
}

func (ts *TerminatedStream) Read(p []byte) (int, error) {
	return ts.stream.Read(p)
}

func (ts *TerminatedStream) Close() error {
	return ts.stream.Close()
}

func getJobLogs(clientset *kubernetes.Clientset, name string) (TerminatedStream, error) {
	pods, err := clientset.CoreV1().Pods("scheduler").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", name),
	})
	if err != nil {
		panic(err)
	}
	if len(pods.Items) == 0 {
		panic(errors.New("pod not found"))
	}
	podLogOptions := &apiv1.PodLogOptions{
		Follow: true,
	}
	podLogRequest := clientset.CoreV1().Pods("scheduler").GetLogs(pods.Items[0].Name, podLogOptions)

	stream, err := podLogRequest.Stream(context.TODO())
	if err != nil {
		panic(err)
	}

	closer := func(stop chan<- interface{}) {
		defer close(stop)
		// wait for the job to finish executing
		// TODO: implement event listener to detect when the job completes
		// this should be blocking code
		namespace := "scheduler"
		factory := informers.NewSharedInformerFactoryWithOptions(clientset, time.Second*10, informers.WithNamespace(namespace))
        // TODO: look at this
		informer := factory.Core().V1().Events().Informer()
		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				event := obj.(*apiv1.Event)
				fmt.Printf("New event: %s\n", event.Message)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				// Handle event update
			},
			DeleteFunc: func(obj interface{}) {
				// Handle event deletion
			},
		})

		stopCh := make(chan struct{})
		defer close(stopCh)

		go informer.Run(stopCh)

		stop <- nil
	}

	return NewTerminatedStream(stream, closer), nil
}
