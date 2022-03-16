document.addEventListener('DOMContentLoaded', () => {
    Highcharts.chart('battPct', {
        chart: {
            type: 'line',
            zoomType: 'x',
            panning: true,
            panKey: 'shift',
        },

        title: {
            text: 'Battery Level'
        },

        xAxis: {
            type: 'datetime',
            title: {
                text: 'Date'
            }
        },

        yAxis: {
            title: {
                text: '%'
            },
            max: 100
        },

        tooltip: {
            headerFormat: '<b>{series.name}</b><br>',
            pointFormat: '{point.x:%H:%M}: {point.y:.2f}%'
        },

        plotOptions: {
            useUTC: false
        },

        series: [{
                name: 'Level',
                data: {{.BatteryGraphData}
    }
}
],


})
    ;
});
