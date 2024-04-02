package fibheap

// TODO: move this to a generic heap test package.

import (
	"context"
	"math/rand/v2"
	"reflect"
	"strconv"
	"testing"

	fibheap "github.com/aperturerobotics/hydra/kvtx/fibheap"
	object_mock "github.com/aperturerobotics/hydra/object/mock"
	"github.com/aperturerobotics/hydra/testbed"
)

func init() {
	testbed.Verbose = false
}

var NumberSequence1 = [...]float64{
	6145466173.743959, 1717075442.6908855, -9223106115.008125,
	6664774768.783949, -9185895273.675707, -2271628840.682966, -6843837387.469989,
	-3075112103.982916, -7315786187.596851, 9022422938.330479, 9230482598.051868,
	-2019031911.3141594, 4852342381.928253, 7767018098.497437, -5163143977.984332,
	7265142312.343864, -9974588724.261246, -4721177341.970384, 6608275091.590723,
	-2509051968.8908787, -2608600569.397663, 4602079812.256586, 4204221071.262924,
	2072073006.576254, -1375445006.5510921, 9753983872.378643, 3379810998.918478,
	-2120599284.15699, -9284902029.588614, 3804069225.763077, 4680667479.457649,
	3550845076.5165443, 689351033.7409191, -6170564101.460268, 5769309548.4711685,
	-7203959673.554039, -1542719821.5259266, 8314666872.8992195, 4582459708.761353,
	4558164249.709116, -409019759.7648945, 2050647646.0881348, 3337347280.2468243,
	8841975976.437397, -1540752999.8368673, 4548535015.628077, -7013783667.095476,
	2287926261.9939594, -2539231979.834078, -9359850979.452446, 5390795464.938633,
	-9969381716.563528, 3273172669.620493, -8839719143.511513, 9436856014.244781,
	9032693590.852093, 748366072.01511, -8165322713.346881, -9745450118.0132,
	-6554663739.562494, -8350123090.830288, 4767099194.408716, -741610722.9710865,
	978853190.937952, -4689006449.5764475, 6712607751.828266, 1834187952.9013042,
	8144068220.835762, 2649156704.6132507, 5206492575.513319, 2355676989.886942,
	6014313651.805082, 1559476573.9042358, -611075813.2161636, -3428570708.324188,
	3758297334.844446, -73880069.57582092, 7939090089.227123, -6135368824.336376,
	5680302744.840729, 7067968530.463007, -4736146992.716046, 6787733005.103142,
	8291261997.956814, -7976948033.245457, -2717662205.411746, 1753831326.4953232,
	3313929049.058649, -6798511690.417229, 4259620288.6441, -8795846089.203701,
	666087815.4947224, -3189108786.1266823, 6098522858.07811, 3670419236.2020073,
	-4904172359.7338295, 7081860835.300518, 4838004130.57917, -8403025837.455175,
	2858604246.067789, 9767232443.473625, 1853770486.2323227, 2111315124.8128128,
	-789990089.2266369, 3855299652.837984, -5262051498.344847, 5195097083.198868,
	-9453697711.29756, -144320772.42621613, -3280154832.042288, 4327603656.616592,
	-4916338352.631529, 177342499.89391518, -6863008836.282527, -4462732551.435464,
	563531299.3931465, 243815563.513546, -2177539298.657405, 9064363201.461056,
	7752407089.025448, 5072315736.623476, 1676308335.832735, 2368433225.444128,
	7191228067.770271, -7952866649.176966, 9029961422.270164, -3694580624.20329,
	2396384720.634838, 2919689806.6469193, 2516309466.887434, 5711191379.798178,
	-7111997035.1143055, -5887152915.558975, 7074496594.814234, 72399466.26899147,
	9162739770.93885, 545095642.1330223, 589248875.6552525, 5429718452.359911,
	2670541446.0850983, 7074768275.337322, -9376701618.064901, -719716639.8418808,
	5870465712.600103, 8906050348.824574, 5260686230.481573, 4525930216.3939705,
	-7558925556.569441, -3524217648.1943235, -8559543174.289785, -402353821.38601303,
	-2939238306.2766924, -8421788462.600799, 173509960.46243477, 2823962320.1096497,
	-2040044596.465724, 8093258879.034134, 1026657583.5726833, -5939324535.959578,
	1869187366.0910244, -8488159448.309237, -9162642241.327745, 9198652822.209103,
	9981219597.001732, 1245929264.1492062, 6333145610.418182, -5007933225.524759,
	-7507006648.70326, -8682109235.019928, 7572534048.487186, 9172777289.492256,
	-4374595711.753318, 7302929281.918972, 6813548014.888256, 7839035144.903576,
	-5126801855.122898, 6523728766.098036, -8063474434.226172, -1011764426.4069233,
	-5468146510.412097, -7725685149.169344, 5224407910.623154, 5337833362.662783,
	3878206583.8412895, -9990847539.012056, 2828249626.7454433, -8802730816.790993,
	-6223950138.847174, -5003095866.683969, 3701841328.9391365, -7438103512.551224,
	-1879515137.467103, -6931067459.813007, -3591253518.1452456, -3249229927.5027523,
	249923973.47061348, -7291235820.978601, -4073015010.864023, -3089932753.657503,
	8220825130.164364,
}

const (
	Seq1FirstMinimum float64 = -9990847539.012056
	Seq1ThirdMinimum float64 = -9969381716.563528
	Seq1FifthMinimum float64 = -9453697711.29756
	Seq1LastMinimum  float64 = 9981219597.001732
)

var NumberSequence2 = [...]float64{
	-2901939070.965906, 4539462982.372177, -6222008480.049856,
	-1400427921.5968666, 9866088144.060883, -2943107648.529664, 8985474333.11443,
	9204710651.257133, 5354113876.8447075, 8122228442.770859, -8121418938.303131,
	538431208.3261185, 9913821013.519611, -8722989752.449871, -3091279426.694975,
	7229910558.195713, -2908838839.99403, 2835257231.305996, 3922059795.3656673,
	-9298869735.322557,
}

const (
	Seq2DecreaseKey1Orig float64 = 9913821013.519611
	Seq2DecreaseKey1Trgt float64 = -8722989752.449871
	Seq2DecreaseKey2Orig float64 = 9866088144.060883
	Seq2DecreaseKey2Trgt float64 = -9698869735.322557
	Seq2DecreaseKey3Orig float64 = 9204710651.257133
	Seq2DecreaseKey3Trgt float64 = -9804710651.257133
)

var NumberSequence2Sorted = [...]float64{
	-9804710651.257133, -9698869735.322557, -9298869735.322557,
	-8722989752.449871, -8722989752.449871, -8121418938.303131, -6222008480.049856,
	-3091279426.694975, -2943107648.529664, -2908838839.99403, -2901939070.965906,
	-1400427921.5968666, 538431208.3261185, 2835257231.305996, 3922059795.3656673,
	4539462982.372177, 5354113876.8447075, 7229910558.195713, 8122228442.770859,
	8985474333.11443,
}

var NumberSequence2Deleted3ElemSorted = [...]float64{
	-9298869735.322557, -8722989752.449871,
	-8121418938.303131, -6222008480.049856, -3091279426.694975, -2943107648.529664,
	-2908838839.99403, -2901939070.965906, -1400427921.5968666, 538431208.3261185,
	2835257231.305996, 3922059795.3656673, 4539462982.372177, 5354113876.8447075,
	7229910558.195713, 8122228442.770859, 8985474333.11443,
}

var NumberSequence3 = [...]float64{
	6015943293.071386, -3878285748.0708866, 8674121166.062424,
	-1528465047.6118088, 7584260716.494843, -373958476.80486107, -6367787695.054295,
	6813992306.719868, 5986097626.907181, 9011134545.052086, 7123644338.268343,
	2646164210.08445, 4407427446.995375, -888196668.2563229, 7973918726.985172,
	-6529216482.09644, 6079069259.51853, -8415952427.784341, -6859960084.757652,
	-502409126.89040375,
}

var NumberSequence4 = [...]float64{
	9241165993.258648, -9423768405.578083, 3280085607.6687145,
	-5253703037.682413, 3858507441.2785892, 9896256282.896187, -9439606732.236805,
	3082628799.5320206, 9453124863.59945, 9928066165.458393, 1135071669.4712334,
	6380353457.986282, 8329064041.853199, 2382910730.445751, -8478491750.445316,
	9607469190.690144, 5417691217.440792, -9698248424.421888, -3933774735.280322,
	-5984555343.381466,
}

var NumberSequenceMerged3And4Sorted = [...]float64{
	-9698248424.421888, -9439606732.236805,
	-9423768405.578083, -8478491750.445316, -8415952427.784341, -6859960084.757652,
	-6529216482.09644, -6367787695.054295, -5984555343.381466, -5253703037.682413,
	-3933774735.280322, -3878285748.0708866, -1528465047.6118088, -888196668.2563229,
	-502409126.89040375, -373958476.80486107, 1135071669.4712334, 2382910730.445751,
	2646164210.08445, 3082628799.5320206, 3280085607.6687145, 3858507441.2785892,
	4407427446.995375, 5417691217.440792, 5986097626.907181, 6015943293.071386,
	6079069259.51853, 6380353457.986282, 6813992306.719868, 7123644338.268343,
	7584260716.494843, 7973918726.985172, 8329064041.853199, 8674121166.062424,
	9011134545.052086, 9241165993.258648, 9453124863.59945, 9607469190.690144,
	9896256282.896187, 9928066165.458393,
}

// assertNoError asserts there was no error.
func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err.Error())
	}
}

// assertEqual asserts two things are equal.
func assertEqual(t *testing.T, t1, t2 interface{}) {
	if !reflect.DeepEqual(t1, t2) {
		t.Fatalf("objects not equal: %#v != %#v", t1, t2)
	}
}

// assertZero asserts something is zero
func assertZero(t *testing.T, t1 interface{}) {
	vo := reflect.ValueOf(t1)
	if s, ok := t1.(string); ok {
		if s != "" {
			t.Fatal("expected string to be empty")
		}
		return
	}
	if i, ok := t1.(float64); ok {
		if i != 0 {
			t.Fatal("expected float to be zero")
		}
		return
	}
	zero := !vo.IsValid() || vo.IsNil()
	if !zero {
		t.Fatalf("expected %v to be zero", t1)
	}
}

func TestSimple(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	// iterating over kv will enqueue in random order
	kv := map[string]float64{
		"test1":    1,
		"test3":    3,
		"test5":    5,
		"test5325": 5325,
		"testNeg2": -2,
	}

	for k, v := range kv {
		assertNoError(t, heap.Enqueue(ctx, []byte(k), v))
	}

	// dequeue in expected order
	order := []float64{-2, 1, 3, 5, 5325}
	for _, expected := range order {
		_, pmin, err := heap.DequeueMin(ctx)
		assertNoError(t, err)
		assertEqual(t, expected, pmin)
	}
}

func TestEnqueueDequeueMin(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	for i := 0; i < len(NumberSequence1); i++ {
		err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), NumberSequence1[i])
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	for {
		size, err := heap.Size(ctx)
		assertNoError(t, err)
		if size == 0 {
			break
		}
		_, minp, err := heap.DequeueMin(ctx)
		assertNoError(t, err)
		size--
		if size == 199 {
			assertEqual(t, Seq1FirstMinimum, minp)
		}
		if size == 197 {
			assertEqual(t, Seq1ThirdMinimum, minp)
		}
		if size == 195 {
			assertEqual(t, Seq1FifthMinimum, minp)
		}
		if size == 0 {
			assertEqual(t, Seq1LastMinimum, minp)
		}
	}
}

func TestFibHeap_Enqueue_Min(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		t.Fatal(err.Error())
	}

	for i := 0; i < len(NumberSequence1); i++ {
		err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), NumberSequence1[i])
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	_, minp, err := heap.Min(ctx)
	assertNoError(t, err)
	assertEqual(t, Seq1FirstMinimum, minp)
}

func TestFibHeap_Min_EmptyHeap(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = heap.Enqueue(ctx, []byte("test"), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	mink, minp, err := heap.DequeueMin(ctx)
	assertNoError(t, err)
	assertEqual(t, float64(0), minp)
	assertEqual(t, "test", string(mink))

	// Heap should be empty at this point
	min, minp, err := heap.Min(ctx)
	assertNoError(t, err)
	assertZero(t, min)
	assertZero(t, minp)
}

func TestEnqueueDecreaseKey(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		t.Fatal(err.Error())
	}

	e1k := "test1"
	e2k := "test2"
	e3k := "test3"
	for i := 0; i < len(NumberSequence2); i++ {
		if NumberSequence2[i] == Seq2DecreaseKey1Orig {
			err := heap.Enqueue(ctx, []byte(e1k), NumberSequence2[i])
			if err != nil {
				t.Fatal(err.Error())
			}
		} else if NumberSequence2[i] == Seq2DecreaseKey2Orig {
			err := heap.Enqueue(ctx, []byte(e2k), NumberSequence2[i])
			if err != nil {
				t.Fatal(err.Error())
			}
		} else if NumberSequence2[i] == Seq2DecreaseKey3Orig {
			err := heap.Enqueue(ctx, []byte(e3k), NumberSequence2[i])
			if err != nil {
				t.Fatal(err.Error())
			}
		} else {
			err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), NumberSequence2[i])
			if err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	err = heap.DecreaseKey(ctx, []byte(e1k), Seq2DecreaseKey1Trgt)
	assertNoError(t, err)
	err = heap.DecreaseKey(ctx, []byte(e2k), Seq2DecreaseKey2Trgt)
	assertNoError(t, err)
	err = heap.DecreaseKey(ctx, []byte(e3k), Seq2DecreaseKey3Trgt)
	assertNoError(t, err)

	for i := 0; i < len(NumberSequence2Sorted); i++ {
		_, minp, err := heap.DequeueMin(ctx)
		assertNoError(t, err)
		assertEqual(t, NumberSequence2Sorted[i], minp)
	}
}

func TestFibHeap_DecreaseKey_EmptyHeap(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	err = heap.Enqueue(ctx, []byte("test"), 15)
	if err != nil {
		t.Fatal(err.Error())
	}
	mink, minp, err := heap.DequeueMin(ctx)
	assertNoError(t, err)
	assertEqual(t, float64(15), minp)
	assertEqual(t, "test", string(mink))

	// Heap should be empty at this point
	err = heap.DecreaseKey(ctx, []byte("test"), 0)
	assertEqual(t, err.Error(), "not found: test")
}

func TestFibHeap_DecreaseKey_LargerNewPriority(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	err = heap.Enqueue(ctx, []byte("test"), 1)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = heap.DecreaseKey(ctx, []byte("test"), 20)
	assertEqual(t, err.Error(), "priority 20 larger than or equal to old: 1")
}

func TestEnqueueDelete(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)
	e1k := "test1"
	e2k := "test2"
	e3k := "test3"
	for i := 0; i < len(NumberSequence2); i++ {
		var err error
		if NumberSequence2[i] == Seq2DecreaseKey1Orig {
			err = heap.Enqueue(ctx, []byte(e1k), NumberSequence2[i])
		} else if NumberSequence2[i] == Seq2DecreaseKey2Orig {
			err = heap.Enqueue(ctx, []byte(e2k), NumberSequence2[i])
		} else if NumberSequence2[i] == Seq2DecreaseKey3Orig {
			err = heap.Enqueue(ctx, []byte(e3k), NumberSequence2[i])
		} else {
			err = heap.Enqueue(ctx, []byte(strconv.Itoa(i)), NumberSequence2[i])
		}
		assertNoError(t, err)
	}

	err = heap.Delete(ctx, []byte(e1k))
	assertNoError(t, err)
	err = heap.Delete(ctx, []byte(e2k))
	assertNoError(t, err)
	err = heap.Delete(ctx, []byte(e3k))
	assertNoError(t, err)

	for i := 0; i < len(NumberSequence2Deleted3ElemSorted); i++ {
		_, pmin, err := heap.DequeueMin(ctx)
		assertNoError(t, err)
		assertEqual(t, NumberSequence2Deleted3ElemSorted[i], pmin)
	}
}

func TestFibHeap_Delete_EmptyHeap(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	err = heap.Enqueue(ctx, []byte("test"), 15)
	assertNoError(t, err)
	_, _, err = heap.DequeueMin(ctx)
	assertNoError(t, err)

	// Heap should be empty at this point
	ie, err := heap.IsEmpty(ctx)
	assertNoError(t, err)
	assertEqual(t, true, ie)
	err = heap.Delete(ctx, []byte("test"))
	assertNoError(t, err)
}

/*
func TestMerge(t *testing.T) {
	objs, tb := object_mock.BuildTestStore(t)
	ctx := context.Background()
	heap1, err := fibheap.NewFibbonaciHeap(objs)
	assertNoError(t, err)

	objs2, err := tb.Volume.OpenObjectStore(context.Background(), "test-2")
	assertNoError(t, err)
	heap2, err := fibheap.NewFibbonaciHeap(objs2)
	assertNoError(t, err)

	for i := 0; i < len(NumberSequence3); i++ {
		heap1.Enqueue(strconv.Itoa(i)+"_1", NumberSequence3[i])
	}
	for i := 0; i < len(NumberSequence4); i++ {
		heap2.Enqueue(strconv.Itoa(i)+"_2", NumberSequence4[i])
	}

	err = heap1.Merge(heap2)
	assertNoError(t, err)

	for i := 0; i < len(NumberSequenceMerged3And4Sorted); i++ {
		_, pmin, err := heap1.DequeueMin()
		assertNoError(t, err)
		assertEqual(t, NumberSequenceMerged3And4Sorted[i], pmin)
	}
}
*/

// Runs in O(1) time
func BenchmarkFibHeap_Enqueue(b *testing.B) {
	objs, _ := object_mock.BuildTestStore(nil)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		panic(err)
	}
	for i := 0; i < b.N; i++ {
		err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), 2*1e10*(rand.Float64()-0.5))
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}

// Runs in O(log(N)) time
func BenchmarkFibHeap_DequeueMin(b *testing.B) {
	objs, _ := object_mock.BuildTestStore(nil)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		panic(err)
	}

	N := 1000000

	slice := make([]float64, 0, N)
	for i := 0; i < N; i++ {
		slice = append(slice, 2*1e10*(rand.Float64()-0.5))
		err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), slice[i])
		if err != nil {
			b.Fatal(err.Error())
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err = heap.DequeueMin(ctx)
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}

// Runs in O(1) amortized time
func BenchmarkFibHeap_DecreaseKey(b *testing.B) {
	objs, _ := object_mock.BuildTestStore(nil)
	ctx := context.Background()
	heap, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		panic(err)
	}

	N := 10000000

	sliceFlt := make([]float64, 0, N)
	for i := 0; i < N; i++ {
		sliceFlt = append(sliceFlt, 2*1e10*(float64(i)-0.5))
		err := heap.Enqueue(ctx, []byte(strconv.Itoa(i)), sliceFlt[i])
		if err != nil {
			b.Fatal(err.Error())
		}
	}

	b.ResetTimer()
	offset := float64(2)
	for i := 0; i < b.N; i++ {
		if i%N == 0 && i > 0 {
			offset *= float64(i / N)
		}
		key := strconv.Itoa(i % N)
		err := heap.DecreaseKey(ctx, []byte(key), sliceFlt[i%N]-offset)
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}

// Runs in O(1) time
/*
func BenchmarkFibHeap_Merge(b *testing.B) {
	objs, tb := object_mock.BuildTestStore(nil)
	ctx := context.Background()
	heap1, err := fibheap.NewFibbonaciHeap(objs)
	if err != nil {
		panic(err)
	}
	objs2, _ := tb.Volume.OpenObjectStore(context.Background(), "test-2")
	heap2, _ := fibheap.NewFibbonaciHeap(objs2)

	for i := 0; i < b.N; i++ {
		heap1.Enqueue(strconv.Itoa(i)+"_1", 2*1E10*(rand.Float64()-0.5))
		heap2.Enqueue(strconv.Itoa(i)+"_2", 2*1E10*(rand.Float64()-0.5))
		err := heap1.Merge(heap2)
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}
*/
